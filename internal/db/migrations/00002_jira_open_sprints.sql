-- +goose Up
-- Задачи, которые сейчас в открытых спринтах (cf[10104] in openSprints()); выставляется полной синхронизацией.
ALTER TABLE jira_issues ADD COLUMN in_open_sprints boolean NOT NULL DEFAULT false;
CREATE INDEX ON jira_issues (project_key) WHERE in_open_sprints;

-- Будущие спринты scrum-досок проекта — кандидаты в «следующий спринт» (reference/sprints/ns_fetch.js).
CREATE TABLE jira_project_future_sprints (
  project_key text   NOT NULL,
  board_id    bigint NOT NULL,
  sprint_id   bigint NOT NULL REFERENCES jira_sprints(id),
  PRIMARY KEY (project_key, sprint_id)
);

-- +goose Down
DROP TABLE jira_project_future_sprints;
ALTER TABLE jira_issues DROP COLUMN in_open_sprints;
