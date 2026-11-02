COMPOSE = docker compose -f deploy/docker-compose.yml --env-file deploy/.env

.PHONY: dev test lint gen migrate up down logs deploy backup tgparse

dev:            ## postgres в docker, api и web локально
	docker run -d --rm --name reports-pg -e POSTGRES_PASSWORD=dev -e POSTGRES_USER=reports -e POSTGRES_DB=reports -p 5432:5432 postgres:16-alpine || true
	DATABASE_URL=postgres://reports:dev@localhost:5432/reports?sslmode=disable go run ./cmd/reports serve & \
	cd web && pnpm dev

test:
	go test ./...
	cd web && pnpm vitest run

tgparse:        ## только парсер Telegram + метрики на корпусе
	go test -v -run 'TestCorpus|TestVariations' ./internal/tgparse/

lint:
	go vet ./...
	golangci-lint run
	cd web && pnpm lint

gen:
	sqlc generate
	oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
	cd web && pnpm openapi-typescript ../api/openapi.yaml -o types/api.d.ts

migrate:
	goose -dir internal/db/migrations postgres "$$DATABASE_URL" up

up:
	$(COMPOSE) up -d --build
down:
	$(COMPOSE) down
logs:
	$(COMPOSE) logs -f --tail=200 api worker
deploy:         ## на сервере: git pull + пересборка
	git pull --ff-only && $(COMPOSE) up -d --build && $(COMPOSE) exec api /app/reports migrate
backup:
	deploy/backup.sh
