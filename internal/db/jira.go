package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/metrics"
)

// LastSync — время окончания последней успешной синхронизации источника (nil, если её не было).
func LastSync(ctx context.Context, pool *pgxpool.Pool, source string) (*time.Time, error) {
	var t *time.Time
	err := pool.QueryRow(ctx, `SELECT max(finished_at) FROM sync_runs WHERE source = $1 AND ok`, source).Scan(&t)
	return t, err
}

// OpenSprintIssues — задачи открытых спринтов со спринтами в порядке Jira.
func OpenSprintIssues(ctx context.Context, pool *pgxpool.Pool) ([]metrics.Issue, error) {
	sprints := map[int64]metrics.Sprint{}
	rows, err := pool.Query(ctx, `SELECT id, name, state, start_date, end_date FROM jira_sprints
		WHERE id IN (SELECT unnest(sprint_ids) FROM jira_issues WHERE in_open_sprints)`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s metrics.Sprint
		if err := rows.Scan(&s.ID, &s.Name, &s.State, &s.Start, &s.End); err != nil {
			rows.Close()
			return nil, err
		}
		sprints[s.ID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = pool.Query(ctx, `SELECT i.key, i.project_key, coalesce(p.name, i.project_key), i.status_id, i.status_name,
			coalesce(i.type, ''), coalesce(i.plan, ''), coalesce(i.assignee, ''), i.created, i.updated, i.timespent_s, i.sprint_ids
		FROM jira_issues i LEFT JOIN jira_projects p ON p.key = i.project_key
		WHERE i.in_open_sprints`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (metrics.Issue, error) {
		var is metrics.Issue
		var ids []int64
		err := r.Scan(&is.Key, &is.ProjectKey, &is.ProjectName, &is.StatusID, &is.StatusName,
			&is.Type, &is.Plan, &is.Assignee, &is.Created, &is.Updated, &is.TimespentS, &ids)
		for _, id := range ids {
			if s, ok := sprints[id]; ok {
				is.Sprints = append(is.Sprints, s)
			}
		}
		return is, err
	})
}

// ProjectLeads — руководитель (лид в Jira) по ключу проекта.
func ProjectLeads(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `SELECT key, coalesce(lead, '—') FROM jira_projects`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, l string
		if err := rows.Scan(&k, &l); err != nil {
			return nil, err
		}
		m[k] = l
	}
	return m, rows.Err()
}

// FutureSprints — будущие спринты досок по проектам.
func FutureSprints(ctx context.Context, pool *pgxpool.Pool) (map[string][]metrics.Sprint, error) {
	rows, err := pool.Query(ctx, `SELECT f.project_key, s.id, s.name, s.state, s.start_date, s.end_date
		FROM jira_project_future_sprints f JOIN jira_sprints s ON s.id = f.sprint_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string][]metrics.Sprint{}
	for rows.Next() {
		var k string
		var s metrics.Sprint
		if err := rows.Scan(&k, &s.ID, &s.Name, &s.State, &s.Start, &s.End); err != nil {
			return nil, err
		}
		m[k] = append(m[k], s)
	}
	return m, rows.Err()
}

// SprintIssueCounts — число задач в каждом активном и будущем спринте (как cf[10104] = id в Jira).
func SprintIssueCounts(ctx context.Context, pool *pgxpool.Pool) (map[int64]int, error) {
	rows, err := pool.Query(ctx, `SELECT s, count(*) FROM jira_issues, unnest(sprint_ids) s
		WHERE in_open_sprints OR in_future_sprints GROUP BY s`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		m[id] = n
	}
	return m, rows.Err()
}
