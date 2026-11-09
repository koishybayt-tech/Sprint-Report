-- +goose Up
-- Уведомление «задачу назначили на вас» (руководителю вернули задачу): колокольчик и личный бот.
ALTER TABLE notify_settings ADD COLUMN on_assigned boolean NOT NULL DEFAULT true;
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('blocked', 'unblock', 'gap', 'assigned', 'test'));

-- +goose Down
DELETE FROM notifications WHERE kind = 'assigned';
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('blocked', 'unblock', 'gap', 'test'));
ALTER TABLE notify_settings DROP COLUMN on_assigned;
