# Архитектура

```
            Telegram group ──(bot webhook)──┐
                                            ▼
Browser ──HTTPS──► Caddy ──/api/*──► api (Go, chi) ──► PostgreSQL 16
                     │                      ▲
                     └──/*──► web (Nuxt 3)  │
                                            │
                         worker (Go) ───────┘
                           ├─ Jira DC  (REST v2, PAT)      каждые 15 мин инкрементально, ночью полный
                           ├─ Bitrix (API отпусков)          каждые 2 ч: отпуска, БС
                           └─ пересчёт агрегатов, снепшоты, Excel
```

## Контейнеры (deploy/docker-compose.yml)
| Сервис | Образ | Порт | Назначение |
|---|---|---|---|
| postgres | postgres:16-alpine | 5432 (внутр.) | данные |
| api | `Dockerfile.api` (`reports serve`) | 8080 | REST + вебхук бота |
| worker | тот же образ (`reports worker`) | — | синхронизации по расписанию |
| web | `Dockerfile.web` | 3000 | Nuxt |
| caddy | caddy:2 | 80/443 | TLS (Let's Encrypt), прокси |

Один Go-бинарь `reports` с подкомандами — проще деплой и общие пакеты.

## Синхронизация Jira (`internal/jira`)
1. Полная: `cf[10104] in openSprints()` + задачи спринтов за последние 180 дней с `expand=changelog`, пачками по 100 (`startAt`), в `jira_issues` (raw JSONB + выделенные колонки), `jira_changelog`, `jira_sprints`.
2. Инкрементальная: `updated >= lastRun − 5 мин`; удалённые из спринтов определяются сравнением множества ключей.
3. Справочники: проекты и лиды (`/project?expand=lead`), тимлиды (`membersOf("adata-team-leads")`), неактивные пользователи (`/user/search?includeInactive=true`, `active=false`).
4. После sync — пересчёт `sprint_daily` (ряд на 18:00) и агрегатов.

## Telegram (`internal/tgbot`)
- Приём сообщений — длинный опрос `getUpdates` в worker (работает без публичного адреса, в том числе локально); `TG_BOT_TOKEN` и `TG_CHAT_ID` в env. Вебхук `/api/v1/tg/webhook` — позже, для сервера.
- В обычной группе нумерация сообщений своя у каждого аккаунта, поэтому сообщения бота и экспорта хранятся раздельно (`tg_messages.source` = bot | export) и сверяются по (автор, время, текст).
- Автор → сотрудник: Telegram ID → @username → ФИО в первых строках (порядок слов и казахские буквы не важны) → имя аккаунта, если однозначно. Найденный по ФИО Telegram ID запоминается в `tg_aliases`. На корпусе: 99,5 % верно, 0 ошибок.
- Для отчёта «Спринты»: больничные (подряд идущие дни, разрыв только на выходные) и «отпросился» (за день ≥ 2 ч или весь день) — без причин, их видят только admin/hr.
- Бот добавлен в группу админом, privacy mode **off** (BotFather → /setprivacy → Disable), иначе бот не видит обычные сообщения.
- Webhook `POST /api/v1/tg/webhook` с заголовком `X-Telegram-Bot-Api-Secret-Token` = `TG_WEBHOOK_SECRET`.
- Обрабатываем `message`, `edited_message`. Сохраняем raw → `tgparse.Parse` → `attendance_events`.
- История до запуска бота: экспорт Telegram Desktop (JSON) → `reports import-telegram --file result.json` (идемпотентно по `message_id`).

## Bitrix (`internal/bitrix`)
Свой API отпусков: `GET $BITRIX_VACATION_URL?id=<табельный ID>&api_key=$BITRIX_API_KEY` (ключ — секрет, только env). Табельный ID — часть email сотрудника в Jira до «@» (`/rest/api/2/user?username=<логин>`), для нетабельных email — колонка `tab_id` в `seed/employees.csv`. Берутся «Трудовой отпуск» → `vacation` и «Отпуск б/с» → `unpaid`, статус «Прерван» пропускается → `absences(source='bitrix')`. Больничные — только из Telegram. Синхронизация — worker раз в `BITRIX_SYNC_EVERY` (2h), вручную `reports sync bitrix`, сырой ответ — `reports bitrix probe --id <табельный ID>`.

## Авторизация
Telegram Login Widget (сотрудник уже есть в `tg_aliases`) или OIDC компании, если появится. Сессия — httpOnly cookie. Роли: `admin`, `lead` (руководители/ПМ/тимлиды), `hr`, `employee`. См. `security.md`.

## Фронт (web/)
Страницы повторяют `design/prototypes/*.html`:
- `/sprints` — все вкладки отчёта «Спринты» (Статус, Ресурсы, Отсутствия, Нужны задачи, Перегружены, …, Качество данных).
- `/attendance` — посещаемость (lead/hr/admin).
- `/employee/:id` — карточка сотрудника.
- `/snapshots`, `/snapshots/:date` — снепшоты к созвонам (неизменяемые).
- `/pm-load` — нагрузка ПМов + кнопка Excel.
- `/admin/review` — ручная очередь разбора Telegram и «Кто это?».
Данные — через сгенерированный из OpenAPI клиент. Графики — vue-echarts. Таблицы — сортировка и фильтры как в прототипах.

## Снепшоты
`reports snapshot --date 2026-10-27` (или кнопка в UI) замораживает все расчёты на дату в `snapshots(payload JSONB)`. Страница снепшота читает только payload, никогда не пересчитывает.
