# Adata Reports — стартовый пакет

Отдельный сервис отчётов Adata: Go (api + worker) + Nuxt 3 + PostgreSQL, Telegram-бот в группе, Jira и Bitrix.

**Репозиторий должен быть приватным** — в `seed/`, `testdata/`, `design/` реальные ФИО и сообщения.

## Что внутри
| Путь | Что |
|---|---|
| `CLAUDE.md` | правила для Claude Code (читает автоматически) |
| `DEPLOY.md` | как поднять сервис на сервере |
| `docs/` | архитектура, разбор Telegram, метрики, безопасность, `schema.sql` |
| `api/openapi.yaml` | контракт API |
| `internal/tgparse/` | **готовый** разбор сообщений группы + тесты на 6 019 реальных сообщениях |
| `testdata/telegram_corpus.jsonl` | корпус с эталонной разметкой |
| `seed/` | сотрудники (93), алиасы Telegram (73), праздники 2026 (проверить названия) |
| `reference/` | текущий код отчётов (JS/Python) — источник формул |
| `design/prototypes/` | текущие HTML-отчёты = макеты страниц |
| `deploy/` | docker-compose, Caddyfile, Dockerfile'ы, `.env.example`, `backup.sh` |

## Как делать в Claude Code
1. Создать приватный репозиторий, распаковать пакет, `git init && git add . && git commit -m "starter"`.
2. Открыть папку в Claude Code. Он прочитает `CLAUDE.md`.
3. Идти по этапам из раздела «План работ» ТЗ, по одному этапу на сессию. Пример первого запроса:
   > Этап 1: каркас. Создай cmd/reports с подкомандами serve/worker/migrate/seed, internal/config, миграцию из docs/schema.sql, sqlc, healthcheck. `make test` и `docker compose up` должны работать.
4. После каждого этапа — проверить критерии приёмки из ТЗ, закоммитить.

Проверка парсера прямо сейчас:
```bash
go test -v ./internal/tgparse/
# messages=6019 type=100.00% absences=5000 ranges=98.30% manual=1.02%
```
