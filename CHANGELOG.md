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
