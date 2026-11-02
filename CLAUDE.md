# Adata Reports — инструкции для Claude Code

Сервис отчётов Adata: спринты Jira, посещаемость (Telegram-группа + Bitrix), карточка сотрудника, снепшоты к созвонам, нагрузка ПМов.
Полное ТЗ — в Claude Doc «Adata Reports — ТЗ сервиса (Go + Nuxt)». Здесь — короткая выжимка правил для работы в репо.

## Стек (не менять без обсуждения)
- **api/worker**: Go 1.23+, chi, pgx v5 + sqlc, goose (миграции), oapi-codegen (из `api/openapi.yaml`), go-telegram/bot, excelize, log/slog.
- **web**: Nuxt 3 + TypeScript, Pinia, Tailwind, vue-echarts. SSR выключен для страниц отчётов (`ssr:false` в routeRules) — это внутренний инструмент.
- **db**: PostgreSQL 16. Всё время — `timestamptz`, отображение в `Asia/Almaty`.
- **deploy**: Docker Compose + Caddy (HTTPS). Файлы — в `deploy/`.

## Структура репо
```
cmd/reports/          # один бинарь: serve | worker | seed | sync | import-telegram | tg
internal/config       # env → struct
internal/db           # sqlc-сгенерированный код + migrations/
internal/jira         # клиент Jira DC (PAT), инкрементальная синхронизация
internal/bitrix       # отпуска и БС из API отпусков Bitrix (vacation_api.php)
internal/tgbot        # вебхук бота, запись raw-сообщений
internal/tgparse      # РАЗБОР СООБЩЕНИЙ — уже написан, покрыт тестами
internal/metrics      # формулы из docs/metrics.md
internal/api          # HTTP-хендлеры (oapi-codegen strict server)
internal/export       # Excel (нагрузка ПМов и т.п.)
web/                  # Nuxt
docs/                 # архитектура, парсинг, метрики, безопасность, schema.sql
reference/            # СТАРЫЙ код (JS/Python), который сейчас считает отчёты. Источник истины по формулам.
design/prototypes/    # готовые HTML-отчёты = макеты страниц. Новый UI должен выглядеть так же.
testdata/             # корпус сообщений Telegram с эталонной разметкой
seed/                 # сотрудники, алиасы Telegram, праздники
```

## Правила
1. **Формулы не придумывать.** Любая метрика берётся из `docs/metrics.md`; если там неясно — читать `reference/` (например, `reference/snapshot/sprint_port.js`, `reference/sprints/inc.js`) и переносить 1-в-1. Расхождение с текущими дашбордами = баг.
2. **Парсер Telegram** (`internal/tgparse`) менять только вместе с тестами. `go test ./internal/tgparse/` должен проходить: тип ≥ 99 %, диапазоны ≥ 97 %, ручная очередь ≤ 3 %. Новый вариант записи → сначала кейс в `TestVariations`, потом код. Поднимать `tgparse.Version` при изменении правил; сохранённые разборы с более старой версией пересчитываются командой `reports reparse`.
3. **Сырые данные храним всегда** (`tg_messages.raw`, `jira_issues.raw`). Всё производное можно пересчитать.
4. **Секреты только в env** (`.env` на сервере, не в git). Ключ Bitrix и Jira PAT никогда не логировать и не возвращать в API.
5. **Персональные данные**: репозиторий приватный (в `seed/` и `testdata/` реальные ФИО). Посещаемость видна только ролям `admin` и `hr` + сам сотрудник — см. `docs/security.md`.
6. **Jira**: Server/DC `jira.adata.kz`, API v2. Поля: sprint `customfield_10104` (в JQL — `cf[10104]`; `sprint in openSprints()` даёт 400), tester `customfield_10902`, план `customfield_11204` («Плановая»/«Внеплановая»), должность `customfield_11101`, epic link `customfield_10100`. Статусы по id: 10100 todo, 10011 в работе, 10008 code review, 10000 rework, 10001 done; прочее = тестирование.
7. **Неактивные пользователи Jira** исключаются из «Нужны задачи».
8. Каждый этап из раздела «План работ» ТЗ заканчивается: тесты зелёные, `make lint` чистый, короткая запись в `CHANGELOG.md`.

## Команды
```
make dev          # postgres в docker + api + web локально
make test         # go test ./... + vitest
make gen          # sqlc + oapi-codegen + типы для web
make migrate      # goose up
go run ./cmd/reports seed --dir seed
go run ./cmd/reports sync jira --full
go run ./cmd/reports import-telegram --file result.json
go run ./cmd/reports tg set-webhook
```
