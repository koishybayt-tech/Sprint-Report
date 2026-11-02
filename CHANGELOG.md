# Changelog

## 2026-11-02 — каркас для запуска
- `cmd/reports`: подкоманды `serve` (chi, `GET /api/v1/healthz` с проверкой БД), `worker` (заглушка, лог раз в минуту), `migrate` (goose, миграции встроены в бинарь), `seed --dir`, `healthcheck`.
- `internal/db/migrations/00001_init.sql` — схема из `docs/schema.sql`.
- `web/`: минимальный Nuxt 3 (pnpm), страница «/» показывает статус API и БД; `/api/**` проксируется на api (`NUXT_API_UPSTREAM`).
- Go 1.26 (этого требуют goose v3.29 и pgx v5.12), Node 22 в `Dockerfile.web`; добавлены `.dockerignore` и `deploy/docker-compose.local.yml` (без Caddy, api :8080, web :3000).
