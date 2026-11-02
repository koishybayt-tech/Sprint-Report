# Changelog

## 2026-11-02 — каркас для запуска
- `cmd/reports`: подкоманды `serve` (chi, `GET /api/v1/healthz` с проверкой БД), `worker` (заглушка, лог раз в минуту), `migrate` (goose, миграции встроены в бинарь), `seed --dir`, `healthcheck`.
- `internal/db/migrations/00001_init.sql` — схема из `docs/schema.sql`.
- `web/`: минимальный Nuxt 3 (pnpm), страница «/» показывает статус API и БД; `/api/**` проксируется на api (`NUXT_API_UPSTREAM`).
- Go 1.26 (этого требуют goose v3.29 и pgx v5.12), Node 22 в `Dockerfile.web`; добавлены `.dockerignore` и `deploy/docker-compose.local.yml` (без Caddy, api :8080, web :3000).

## 2026-11-02 — Спринты, этап 1: «Статус спринтов»
- `reports sync jira [--full]`: задачи активных (`openSprints()`) и будущих (`futureSprints()`) спринтов с сырым JSON, проекты с лидами, будущие спринты scrum-досок; запуски в `sync_runs`. Миграции `00002`, `00003`.
- `internal/metrics`: строка проекта 1-в-1 по `reference/sprints/inc.js` и `page_template.html`, выбор следующего спринта по `ns_fetch.js`; тесты.
- API: `GET /api/v1/sprints?lead=`, `GET /api/v1/sprints/next`.
- web: `/sprints` — вкладка «Статус спринтов» (KPI, фильтр по руководителю, сортировка, ссылки в Jira) в стилях текущего дашборда; `/` ведёт на `/sprints`, проверка API — `/status`.

## 2026-11-02 — Спринты, этап 2: проектные вкладки и автосинхронизация
- worker синхронизирует Jira при старте и каждые `JIRA_SYNC_EVERY` (15m); без `JIRA_PAT` синхронизация выключена.
- Синк забирает тимлидов (`membersOf("adata-team-leads")`, миграция `00004`).
- `internal/metrics/report.go`: агрегаты как в `inc.js` (люди × проекты, тестировщики, узкие места, должности, логины), `tst_fetch.js` (Test по тестировщикам), `ns_fetch.js` (статусы и исполнители следующего спринта); `GET /api/v1/sprints/report`.
- web: вкладки «Загрузка QA», «Узкие места», «Стабильность», «Качество», «Готовность к релизу», «Следующий спринт», «Качество данных»; общий `SortTable`; `vue-tsc` для проверки типов.

## 2026-11-02 — Спринты, этап 3: вкладки по людям и дашборд
- Отчёт отдаёт неактивных пользователей Jira (флаг `active` исполнителя) — исключаются из «Нужны задачи» и «Ресурсов по руководителям».
- web: «Дэшборды» (сводка по руководителям, кому нужны задачи), «Загрузка ресурсов», «Ресурсы по руководителям» (с раскрытием по проектам), «Нужны задачи», «Перегружены», «Переключаются между проектами» (с подсказкой «почему»), «Тимлиды и бэклог»; KPI по людям. Логика — `web/utils/sprintsPeople.ts`, 1-в-1 по `page_template.html`.
- Вкладка «Отсутствия» и колонки об отсутствиях — после подключения Bitrix и Telegram.

## 2026-11-02 — Bitrix: API отпусков
- Вместо `BITRIX_WEBHOOK_URL` — `BITRIX_VACATION_URL` (https://bitrix.avh.kz/api/v1/vacation_api.php) и `BITRIX_API_KEY`; конфиг, `.env.example`, docs.
- Табельный ID: часть email сотрудника в Jira до «@», для нетабельных email — колонка `tab_id` в `seed/employees.csv` (сейчас нужна vbakeev, adavletov). Миграция `00005`.
- `internal/bitrix`: запрос по каждому сотруднику, «Трудовой отпуск» → vacation, «Отпуск б/с» → unpaid, «Прерван» и больничные пропускаются; запись в `absences` (source=bitrix) с дедупом по external_id. Ключ маскируется в логах, ошибках и выводе.
- `reports bitrix probe --id <ID>` — сырой ответ API; `reports sync bitrix`; worker синхронизирует раз в `BITRIX_SYNC_EVERY` (2h).
- Разбор по настоящему ответу (`items[]`: `element_id`, `vocation_type`, `date_start`/`date_end` в ДД.ММ.ГГГГ, `status`); 404 «отпуска не найдены» = заявлений нет; в статистике синка — все встретившиеся типы и статусы.
