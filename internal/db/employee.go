package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/metrics"
)

// EmpIssues — задачи, обновлявшиеся с начала года (по Алматы), со спринтами и историей статуса/исполнителя.
// Автор и тестировщик — логины из raw (в колонках — отображаемые имена).
func EmpIssues(ctx context.Context, pool *pgxpool.Pool) ([]metrics.EmpIssue, error) {
	type sp struct {
		state      string
		start, end *time.Time
	}
	sprints := map[int64]sp{}
	srows, err := pool.Query(ctx, `SELECT id, state, start_date, end_date FROM jira_sprints`)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var id int64
		var s sp
		if err := srows.Scan(&id, &s.state, &s.start, &s.end); err != nil {
			srows.Close()
			return nil, err
		}
		sprints[id] = s
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return nil, err
	}

	const yearStart = `date_trunc('year', now() AT TIME ZONE 'Asia/Almaty') AT TIME ZONE 'Asia/Almaty'`
	rows, err := pool.Query(ctx, `SELECT key, coalesce(summary, ''), coalesce(type, ''), coalesce(status_name, ''),
			coalesce(raw->'fields'->'status'->'statusCategory'->>'key', '') = 'done',
			coalesce(assignee_login, ''), coalesce(raw->'fields'->'reporter'->>'name', ''),
			coalesce(raw->'fields'->'customfield_10902'->>'name', ''),
			created, resolved, timespent_s, sprint_ids
		FROM jira_issues WHERE updated >= `+yearStart)
	if err != nil {
		return nil, err
	}
	var out []metrics.EmpIssue
	idx := map[string]int{}
	for rows.Next() {
		var is metrics.EmpIssue
		var ids []int64
		if err := rows.Scan(&is.Key, &is.Summary, &is.Type, &is.Status, &is.Done, &is.Assignee, &is.Reporter, &is.Tester,
			&is.Created, &is.Resolved, &is.TimespentS, &ids); err != nil {
			rows.Close()
			return nil, err
		}
		for _, id := range ids {
			if s, ok := sprints[id]; ok {
				is.Sprints = append(is.Sprints, metrics.EmpSprint{State: s.state, Start: s.start, End: s.end})
			}
		}
		idx[is.Key] = len(out)
		out = append(out, is)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	crows, err := pool.Query(ctx, `SELECT c.issue_key, c.at, c.field, coalesce(c.from_val, ''), coalesce(c.to_val, '')
		FROM jira_changelog c JOIN jira_issues i ON i.key = c.issue_key
		WHERE i.updated >= `+yearStart+` ORDER BY c.at`)
	if err != nil {
		return nil, err
	}
	defer crows.Close()
	for crows.Next() {
		var key string
		var c metrics.EmpChange
		if err := crows.Scan(&key, &c.At, &c.Field, &c.From, &c.To); err != nil {
			return nil, err
		}
		if i, ok := idx[key]; ok {
			out[i].Changes = append(out[i].Changes, c)
		}
	}
	return out, crows.Err()
}

// EmpPeople — справочник сотрудников для карточки (роль и тимлид дополняются из отчёта по спринтам).
func EmpPeople(ctx context.Context, pool *pgxpool.Pool) ([]metrics.EmpPerson, error) {
	rows, err := pool.Query(ctx, `SELECT id, full_name, coalesce(jira_login, ''), management FROM employees ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []metrics.EmpPerson
	for rows.Next() {
		var p metrics.EmpPerson
		if err := rows.Scan(&p.ID, &p.Name, &p.Login, &p.Lead); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// HistoryLoadedAt — конец последней успешной загрузки истории Jira (nil — ещё не было).
func HistoryLoadedAt(ctx context.Context, pool *pgxpool.Pool) (*time.Time, error) {
	var t *time.Time
	err := pool.QueryRow(ctx, `SELECT max(finished_at) FROM sync_runs WHERE source = 'jira_history' AND ok`).Scan(&t)
	return t, err
}
