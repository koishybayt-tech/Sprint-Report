-- +goose Up
-- Руководитель пользователя: он видит подчинённых (и их подчинённых) и их проекты в «Контроле спринта руководителей».
-- NULL — руководителя нет. Заблокированный руководитель подчинённых не теряет, но своих людей он больше не видит.
ALTER TABLE users ADD COLUMN manager_id bigint REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE users ADD CONSTRAINT users_manager_not_self CHECK (manager_id IS NULL OR manager_id <> id);
CREATE INDEX users_manager_idx ON users (manager_id) WHERE manager_id IS NOT NULL;

-- +goose Down
DROP INDEX users_manager_idx;
ALTER TABLE users DROP CONSTRAINT users_manager_not_self;
ALTER TABLE users DROP COLUMN manager_id;
