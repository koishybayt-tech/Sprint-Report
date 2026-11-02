-- +goose Up
-- Табельный ID для API отпусков Bitrix: по умолчанию — часть email в Jira до «@» (jira_email),
-- для нетабельных email задаётся вручную в seed/employees.csv (tab_id).
ALTER TABLE employees ADD COLUMN tab_id text UNIQUE;
ALTER TABLE employees ADD COLUMN jira_email text;

-- +goose Down
ALTER TABLE employees DROP COLUMN jira_email;
ALTER TABLE employees DROP COLUMN tab_id;
