-- +goose Up
-- Связи задач Jira (issuelinks): нужны «Контролю спринта руководителей» (кто кого ждёт, пара дизайн → фронт/бэк).
CREATE TABLE jira_issue_links (
  issue_key   text NOT NULL REFERENCES jira_issues(key) ON DELETE CASCADE,
  link_type   text NOT NULL,          -- Relate | Blocks | Has to be done | Clone | Duplicate ...
  direction   text NOT NULL CHECK (direction IN ('outward', 'inward')),
  phrase      text NOT NULL,          -- как читается от этой задачи: relates to, is blocked by, ...
  linked_key  text NOT NULL,
  PRIMARY KEY (issue_key, link_type, direction, linked_key)
);
CREATE INDEX ON jira_issue_links (linked_key);

-- Руководитель → его проекты (docs/lead-control.md); редактируется в «Настройках».
CREATE TABLE lead_projects (
  lead         text NOT NULL,           -- ФИО руководителя, как в справочнике сотрудников и Jira
  project_key  text NOT NULL,
  title        text NOT NULL DEFAULT '', -- короткое название для чипа: «Комплаенс»
  position     int  NOT NULL DEFAULT 0,  -- порядок чипов
  PRIMARY KEY (lead, project_key)
);

-- Готовый расчёт модуля по руководителю: пересчитывается worker'ом после каждой синхронизации Jira.
CREATE TABLE lead_control (
  lead         text PRIMARY KEY,
  payload      jsonb NOT NULL,
  computed_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE lead_control, lead_projects, jira_issue_links;
