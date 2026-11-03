-- +goose Up
-- Доступ пользователей к модулям из шапки (sprints, lead_control, attendance, employees).
-- Существующие пользователи получают все модули; ограничения по роли (docs/security.md) остаются в силе.
ALTER TABLE users ADD COLUMN modules text[] NOT NULL DEFAULT '{sprints,lead_control,attendance,employees}';

-- +goose Down
ALTER TABLE users DROP COLUMN modules;
