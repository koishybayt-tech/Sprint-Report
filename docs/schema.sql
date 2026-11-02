-- Adata Reports — начальная схема (goose: internal/db/migrations/00001_init.sql)
-- +goose Up

CREATE TABLE employees (
  id            bigserial PRIMARY KEY,
  full_name     text NOT NULL UNIQUE,
  jira_login    text UNIQUE,
  position      text,                 -- Frontend / Backend / QA Engineer / Project Manager ...
  team_lead     text,
  project_lead  text,
  staff         boolean NOT NULL DEFAULT true,   -- в штате (не подрядчик)
  left_at       date,                 -- уволен
  jira_active   boolean NOT NULL DEFAULT true,   -- false → исключать из «Нужны задачи»
  role          text NOT NULL DEFAULT 'employee' CHECK (role IN ('admin','lead','hr','employee')),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tg_aliases (
  id           bigserial PRIMARY KEY,
  employee_id  bigint NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
  tg_user_id   bigint UNIQUE,
  tg_username  text,
  tg_display   text,
  UNIQUE (tg_username)
);

CREATE TABLE holidays (
  day   date PRIMARY KEY,
  name  text NOT NULL,
  kind  text NOT NULL DEFAULT 'holiday' CHECK (kind IN ('holiday','transfer_off','transfer_work'))
);

-- ---------- Jira ----------
CREATE TABLE jira_projects (
  key        text PRIMARY KEY,
  name       text NOT NULL,
  lead       text
);

CREATE TABLE jira_sprints (
  id          bigint PRIMARY KEY,
  name        text NOT NULL,
  state       text NOT NULL,          -- ACTIVE | FUTURE | CLOSED
  start_date  timestamptz,
  end_date    timestamptz,
  complete_date timestamptz,
  project_key text REFERENCES jira_projects(key)
);

CREATE TABLE jira_issues (
  key          text PRIMARY KEY,
  project_key  text NOT NULL,
  summary      text,
  type         text,
  status_id    text,
  status_name  text,
  status_group text,                  -- todo|wk|cr|rw|ts|dn
  assignee     text,
  assignee_login text,
  reporter     text,
  tester       text,                  -- cf 10902
  plan         text,                  -- cf 11204: Плановая | Внеплановая
  position     text,                  -- cf 11101
  epic_key     text,                  -- cf 10100
  priority     text,
  created      timestamptz NOT NULL,
  updated      timestamptz NOT NULL,
  resolved     timestamptz,
  timespent_s  integer NOT NULL DEFAULT 0,
  sprint_ids   bigint[] NOT NULL DEFAULT '{}',
  raw          jsonb NOT NULL,
  synced_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON jira_issues (project_key);
CREATE INDEX ON jira_issues (assignee);
CREATE INDEX ON jira_issues USING gin (sprint_ids);

CREATE TABLE jira_changelog (
  issue_key  text NOT NULL REFERENCES jira_issues(key) ON DELETE CASCADE,
  at         timestamptz NOT NULL,
  field      text NOT NULL,           -- status | assignee | Sprint
  from_val   text,
  to_val     text,
  author     text,
  PRIMARY KEY (issue_key, at, field)
);

CREATE TABLE sprint_daily (            -- ряд для графика спринта, срез в 18:00
  sprint_id     bigint NOT NULL REFERENCES jira_sprints(id),
  day           date NOT NULL,
  todo_lead     int NOT NULL, todo_unassigned int NOT NULL, todo_other int NOT NULL,
  in_progress   int NOT NULL, done int NOT NULL,
  PRIMARY KEY (sprint_id, day)
);

CREATE TABLE sync_runs (
  id         bigserial PRIMARY KEY,
  source     text NOT NULL,           -- jira|bitrix|telegram
  mode       text NOT NULL,           -- full|inc
  started_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz,
  ok         boolean,
  stats      jsonb,
  error      text
);

-- ---------- Telegram / посещаемость ----------
CREATE TABLE tg_messages (
  chat_id      bigint NOT NULL,
  message_id   bigint NOT NULL,
  from_id      bigint,
  from_name    text,
  sent_at      timestamptz NOT NULL,
  edited_at    timestamptz,
  text         text,
  raw          jsonb NOT NULL,
  employee_id  bigint REFERENCES employees(id),
  parse_version text,
  parse_result jsonb,                 -- tgparse.Result
  parse_status text,                  -- ok|manual|ignored
  PRIMARY KEY (chat_id, message_id)
);
CREATE INDEX ON tg_messages (sent_at);
CREATE INDEX ON tg_messages (employee_id, sent_at);

CREATE TABLE attendance_events (
  id           bigserial PRIMARY KEY,
  employee_id  bigint NOT NULL REFERENCES employees(id),
  day          date NOT NULL,
  kind         text NOT NULL CHECK (kind IN ('late','early','mid','day','sick','vac','unpaid','arrived')),
  from_min     int,
  to_min       int,
  minutes      int NOT NULL DEFAULT 0,
  unpaid       boolean NOT NULL DEFAULT false,
  reason       text,
  reason_cat   text,
  makeup_kind  text,
  makeup_text  text,
  makeup_date  date,
  approved_by  text[],
  source       text NOT NULL CHECK (source IN ('telegram','bitrix','manual')),
  chat_id      bigint, message_id bigint,
  superseded   boolean NOT NULL DEFAULT false,
  created_by   bigint REFERENCES employees(id),   -- для manual
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON attendance_events (employee_id, day) WHERE NOT superseded;
CREATE INDEX ON attendance_events (day) WHERE NOT superseded;

CREATE TABLE absences (                -- многодневные: отпуск, БС, больничный
  id           bigserial PRIMARY KEY,
  employee_id  bigint NOT NULL REFERENCES employees(id),
  kind         text NOT NULL CHECK (kind IN ('vacation','unpaid','sick')),
  date_from    date NOT NULL,
  date_to      date NOT NULL,
  source       text NOT NULL CHECK (source IN ('bitrix','telegram','manual')),
  external_id  text,
  note         text,
  UNIQUE (source, external_id)
);

-- ---------- снепшоты, аудит, сессии ----------
CREATE TABLE snapshots (
  id          bigserial PRIMARY KEY,
  taken_for   date NOT NULL UNIQUE,
  history_end date NOT NULL,
  payload     jsonb NOT NULL,
  note        text,
  created_by  bigint REFERENCES employees(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
  id          bigserial PRIMARY KEY,
  at          timestamptz NOT NULL DEFAULT now(),
  actor_id    bigint REFERENCES employees(id),
  action      text NOT NULL,          -- view_attendance | view_employee | edit_event | role_change ...
  target      text,
  details     jsonb
);

CREATE TABLE sessions (
  token_hash  bytea PRIMARY KEY,
  employee_id bigint NOT NULL REFERENCES employees(id) ON DELETE CASCADE,
  expires_at  timestamptz NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE sessions, audit_log, snapshots, absences, attendance_events, tg_messages,
  sync_runs, sprint_daily, jira_changelog, jira_issues, jira_sprints, jira_projects,
  holidays, tg_aliases, employees;
