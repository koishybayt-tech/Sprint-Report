-- +goose Up
-- Уведомления «Контроля спринта руководителей»: колокольчик в сервисе + личный Telegram-бот руководителя.
CREATE TABLE notifications (
  id          bigserial PRIMARY KEY,
  user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind        text NOT NULL CHECK (kind IN ('blocked', 'unblock', 'gap', 'test')),
  lead        text NOT NULL DEFAULT '',
  issue_key   text NOT NULL DEFAULT '',
  text        text NOT NULL,
  url         text NOT NULL DEFAULT '',   -- куда ведёт уведомление (Jira)
  create_url  text NOT NULL DEFAULT '',   -- «Создать продолжение» (форма Jira) — для kind = gap
  created_at  timestamptz NOT NULL DEFAULT now(),
  read_at     timestamptz
);
CREATE INDEX ON notifications (user_id, created_at DESC);

-- Настройки уведомлений пользователя. Токен бота — секрет: хранится зашифрованным (SECRETS_KEY), в API не отдаётся.
CREATE TABLE notify_settings (
  user_id        bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  on_blocked     boolean NOT NULL DEFAULT true,   -- задача попала в Blocked
  on_unblock     boolean NOT NULL DEFAULT true,   -- задачу можно вытащить из блока
  on_gap         boolean NOT NULL DEFAULT true,   -- задача завершилась без продолжения
  tg_token_enc   bytea,
  tg_bot         text,                            -- @username бота
  tg_chat_id     bigint,                          -- личный чат руководителя с его ботом (после /start <код>)
  tg_link_code   text,                            -- одноразовый код привязки
  tg_offset      bigint NOT NULL DEFAULT 0,       -- getUpdates этого бота
  updated_at     timestamptz NOT NULL DEFAULT now()
);

-- Что уже было по руководителю: новые записи относительно этого состояния и есть события.
CREATE TABLE lead_notify_state (
  lead       text NOT NULL,
  kind       text NOT NULL,
  issue_key  text NOT NULL,
  seen_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (lead, kind, issue_key)
);

-- «Игнорировать»: продолжение не нужно — задача скрыта из «Разрывов» и больше не уведомляет.
CREATE TABLE lead_gap_ignored (
  lead       text NOT NULL,
  issue_key  text NOT NULL,
  user_id    bigint REFERENCES users(id) ON DELETE SET NULL,
  at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (lead, issue_key)
);

-- +goose Down
DROP TABLE lead_gap_ignored, lead_notify_state, notify_settings, notifications;
