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
			coalesce(i.type, ''), coalesce(i.plan, ''), coalesce(i.assignee, ''), i.created, i.updated, i.timespent_s, i.sprint_ids,
			coalesce(i.assignee_login, ''), coalesce(i.tester, ''), coalesce(i.raw->'fields'->'customfield_10902'->>'name', ''),
			coalesce(i.position, ''), coalesce(i.summary, ''), coalesce(i.priority, ''),
			coalesce(i.raw->'fields'->'assignee'->>'active' = 'false', false)
		FROM jira_issues i LEFT JOIN jira_projects p ON p.key = i.project_key
		WHERE i.in_open_sprints`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (metrics.Issue, error) {
		var is metrics.Issue
		var ids []int64
		err := r.Scan(&is.Key, &is.ProjectKey, &is.ProjectName, &is.StatusID, &is.StatusName,
			&is.Type, &is.Plan, &is.Assignee, &is.Created, &is.Updated, &is.TimespentS, &ids,
			&is.AssigneeLogin, &is.Tester, &is.TesterLogin, &is.Position, &is.Summary, &is.Priority, &is.AssigneeInactive)
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

// SprintIssues — задачи активных и будущих спринтов (для разбора следующего спринта, как cf[10104] = id).
func SprintIssues(ctx context.Context, pool *pgxpool.Pool) ([]metrics.SprintIssue, error) {
	rows, err := pool.Query(ctx, `SELECT sprint_ids, status_name, coalesce(assignee, '') FROM jira_issues
		WHERE in_open_sprints OR in_future_sprints`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (metrics.SprintIssue, error) {
		var si metrics.SprintIssue
		err := r.Scan(&si.SprintIDs, &si.Status, &si.Assignee)
		return si, err
	})
}

// TeamLeads — тимлиды (группа adata-team-leads), по алфавиту.
func TeamLeads(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT name FROM jira_team_leads ORDER BY name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// Absences — отсутствия, которые ещё не закончились на дату day (или без даты окончания).
func Absences(ctx context.Context, pool *pgxpool.Pool, day time.Time) ([]metrics.AbsenceRow, error) {
	rows, err := pool.Query(ctx, `SELECT coalesce(e.jira_login, ''), e.full_name, a.kind, a.date_from, a.date_to, a.source, coalesce(a.note, '')
		FROM absences a JOIN employees e ON e.id = a.employee_id
		WHERE a.date_to IS NULL OR a.date_to >= $1
		ORDER BY a.date_from, e.full_name`, day)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (metrics.AbsenceRow, error) {
		var a metrics.AbsenceRow
		err := r.Scan(&a.Login, &a.FullName, &a.Kind, &a.From, &a.To, &a.Source, &a.Note)
		return a, err
	})
}

// LastSyncs — время последней успешной синхронизации по каждому источнику.
func LastSyncs(ctx context.Context, pool *pgxpool.Pool) (map[string]time.Time, error) {
	rows, err := pool.Query(ctx, `SELECT source, max(finished_at) FROM sync_runs WHERE ok GROUP BY source`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]time.Time{}
	for rows.Next() {
		var s string
		var t time.Time
		if err := rows.Scan(&s, &t); err != nil {
			return nil, err
		}
		m[s] = t
	}
	return m, rows.Err()
}

// TelegramEvents — события Telegram для отсутствий в отчёте: больничные и опоздания/уходы/отлучки с даты from.
func TelegramEvents(ctx context.Context, pool *pgxpool.Pool, from time.Time) ([]metrics.TgEvent, error) {
	rows, err := pool.Query(ctx, `SELECT coalesce(e.jira_login, ''), e.full_name, a.day, a.kind, a.from_min, a.to_min, a.minutes
		FROM attendance_events a JOIN employees e ON e.id = a.employee_id
		WHERE a.source = 'telegram' AND NOT a.superseded AND a.kind IN ('sick','late','early','mid','day') AND a.day >= $1
		ORDER BY e.full_name, a.day`, from)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (metrics.TgEvent, error) {
		var e metrics.TgEvent
		err := r.Scan(&e.Login, &e.FullName, &e.Day, &e.Kind, &e.FromMin, &e.ToMin, &e.Minutes)
		return e, err
	})
}

// LastTelegramMessage — время последнего сообщения группы (ABSAT.tg в старом отчёте).
func LastTelegramMessage(ctx context.Context, pool *pgxpool.Pool) (*time.Time, error) {
	var t *time.Time
	err := pool.QueryRow(ctx, `SELECT max(sent_at) FROM tg_messages`).Scan(&t)
	return t, err
}
