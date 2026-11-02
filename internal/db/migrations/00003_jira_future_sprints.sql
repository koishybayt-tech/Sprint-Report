-- +goose Up
-- Задачи будущих спринтов (cf[10104] in futureSprints()): openSprints() отдаёт только активные,
-- а «следующий спринт» считает свои задачи, как ns_fetch.js (cf[10104] = id).
ALTER TABLE jira_issues ADD COLUMN in_future_sprints boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE jira_issues DROP COLUMN in_future_sprints;
