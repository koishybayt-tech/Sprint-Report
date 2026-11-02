-- +goose Up
-- Пользователи сервиса (вход по логину и паролю; позже к ним привяжется вход через Telegram).
-- Роли и права — docs/security.md. Пароль — только bcrypt-хеш.
CREATE TABLE users (
  id             bigserial PRIMARY KEY,
  login          text NOT NULL,
  full_name      text NOT NULL,
  role           text NOT NULL CHECK (role IN ('admin','lead','hr','employee')),
  employee_id    bigint REFERENCES employees(id) ON DELETE SET NULL,
  password_hash  text NOT NULL,
  active         boolean NOT NULL DEFAULT true,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  last_login_at  timestamptz
);
CREATE UNIQUE INDEX users_login_key ON users (lower(login));

-- Сессии теперь принадлежат пользователю, а не сотруднику. Таблица ещё не использовалась.
DROP TABLE sessions;
CREATE TABLE sessions (
  token_hash   bytea PRIMARY KEY,           -- sha256 токена из cookie; сам токен не храним
  user_id      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at   timestamptz NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON sessions (user_id);

-- +goose Down
DROP TABLE sessions;
CREATE TABLE sessions (
  token_hash  bytea PRIMARY KEY,
  employee_id bigint NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
  expires_at  timestamptz NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
DROP TABLE users;
