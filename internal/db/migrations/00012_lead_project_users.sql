-- +goose Up
-- Проект «Контроля спринта» можно назначить пользователю сервиса (например, ПМу проекта): он видит его как свой,
-- независимо от того, кто руководитель в Jira. NULL — руководитель только по имени из Jira (как раньше).
ALTER TABLE lead_projects ADD COLUMN user_id bigint REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX lead_projects_user_idx ON lead_projects (user_id) WHERE user_id IS NOT NULL;

-- +goose Down
DROP INDEX lead_projects_user_idx;
ALTER TABLE lead_projects DROP COLUMN user_id;
