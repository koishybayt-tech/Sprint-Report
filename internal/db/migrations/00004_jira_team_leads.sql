-- +goose Up
-- Тимлиды: исполнители задач из группы adata-team-leads за 180 дней (как TL в reference/sprints/inc.js).
CREATE TABLE jira_team_leads (
  name text PRIMARY KEY
);

-- +goose Down
DROP TABLE jira_team_leads;
