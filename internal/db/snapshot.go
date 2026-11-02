package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/metrics"
)

// JiraNames — логин → отображаемое имя Jira (исполнители, авторы задач, справочник сотрудников).
func JiraNames(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT jira_login, full_name FROM employees WHERE jira_login IS NOT NULL
		UNION ALL
		SELECT DISTINCT raw->'fields'->'reporter'->>'name', reporter FROM jira_issues WHERE reporter IS NOT NULL
		UNION ALL
		SELECT DISTINCT assignee_login, assignee FROM jira_issues WHERE assignee_login IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var l, n *string
		if err := rows.Scan(&l, &n); err != nil {
			return nil, err
		}
		if l != nil && n != nil && *l != "" {
			m[*l] = *n // позже — имена из Jira, они главнее справочника
		}
	}
	return m, rows.Err()
}

// SnapIssues — задачи года (как для карточки сотрудника) для снепшота; логины в истории заменены именами Jira.
func SnapIssues(ctx context.Context, pool *pgxpool.Pool, names map[string]string) ([]metrics.SnapIssue, error) {
	sprints := map[int64]metrics.SnapSprint{}
	srows, err := pool.Query(ctx, `SELECT id, name, state,
			coalesce(to_char(start_date AT TIME ZONE 'Asia/Almaty', 'YYYY-MM-DD'), ''),
			coalesce(to_char(end_date AT TIME ZONE 'Asia/Almaty', 'YYYY-MM-DD'), '') FROM jira_sprints`)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var id int64
		var s metrics.SnapSprint
		if err := srows.Scan(&id, &s.Name, &s.State, &s.Start, &s.End); err != nil {
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
	rows, err := pool.Query(ctx, `SELECT key, coalesce(type, ''), coalesce(status_name, ''), coalesce(status_id, ''),
			coalesce(raw->'fields'->'status'->'statusCategory'->>'key', '') = 'done',
			coalesce(assignee, ''), coalesce(reporter, ''), created, resolved, sprint_ids
		FROM jira_issues WHERE updated >= `+yearStart+` OR in_open_sprints OR in_future_sprints`)
	if err != nil {
		return nil, err
	}
	var out []metrics.SnapIssue
	idx := map[string]int{}
	for rows.Next() {
		var is metrics.SnapIssue
		var ids []int64
		if err := rows.Scan(&is.Key, &is.Type, &is.Status, &is.StatusID, &is.Done, &is.Assignee, &is.Reporter,
			&is.Created, &is.Resolved, &ids); err != nil {
			rows.Close()
			return nil, err
		}
		for _, id := range ids {
			if s, ok := sprints[id]; ok {
				is.Sprints = append(is.Sprints, s)
			}
		}
		idx[is.Key] = len(out)
		out = append(out, is)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	name := func(l string) string {
		if n, ok := names[l]; ok {
			return n
		}
		return l
	}
	crows, err := pool.Query(ctx, `SELECT issue_key, at, field, coalesce(from_val, ''), coalesce(to_val, ''), coalesce(author, '')
		FROM jira_changelog ORDER BY at`)
	if err != nil {
		return nil, err
	}
	defer crows.Close()
	for crows.Next() {
		var key string
		var c metrics.SnapChange
		if err := crows.Scan(&key, &c.At, &c.Field, &c.From, &c.To, &c.Author); err != nil {
			return nil, err
		}
		i, ok := idx[key]
		if !ok {
			continue
		}
		c.Author = name(c.Author)
		if c.Field == "assignee" {
			c.From, c.To = name(c.From), name(c.To)
		}
		out[i].Changes = append(out[i].Changes, c)
	}
	return out, crows.Err()
}

// DaysOff — нерабочие дни (праздники и переносы выходных); рабочие субботы-переносы сюда не входят.
func DaysOff(ctx context.Context, pool *pgxpool.Pool) (map[string]bool, error) {
	rows, err := pool.Query(ctx, `SELECT to_char(day, 'YYYY-MM-DD') FROM holidays WHERE kind <> 'transfer_work'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]bool{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		m[d] = true
	}
	return m, rows.Err()
}

// StaffPositions — сотрудники в штате (не уволены) с логином Jira и должностью из справочника.
func StaffPositions(ctx context.Context, pool *pgxpool.Pool) (map[string]string, error) {
	rows, err := pool.Query(ctx, `SELECT jira_login, coalesce(position, '') FROM employees
		WHERE staff AND left_at IS NULL AND jira_login IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var l, p string
		if err := rows.Scan(&l, &p); err != nil {
			return nil, err
		}
		m[l] = p
	}
	return m, rows.Err()
}

type SnapshotInfo struct {
	TakenFor   string    `json:"taken_for"`
	HistoryEnd string    `json:"history_end"`
	Note       string    `json:"note"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}

var ErrSnapshotExists = errors.New("снепшот на эту дату уже есть")

// SaveSnapshot замораживает расчёт; снепшоты неизменяемы — повторный на ту же дату → ErrSnapshotExists.
func SaveSnapshot(ctx context.Context, pool *pgxpool.Pool, takenFor, histEnd string, payload any, note string, by *int64) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `INSERT INTO snapshots (taken_for, history_end, payload, note, created_by)
		VALUES ($1, $2, $3, nullif($4, ''), $5)`, takenFor, histEnd, b, note, by)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return ErrSnapshotExists
	}
	return err
}

func ListSnapshots(ctx context.Context, pool *pgxpool.Pool) ([]SnapshotInfo, error) {
	rows, err := pool.Query(ctx, `SELECT to_char(s.taken_for, 'YYYY-MM-DD'), to_char(s.history_end, 'YYYY-MM-DD'),
			coalesce(s.note, ''), coalesce(e.full_name, ''), s.created_at
		FROM snapshots s LEFT JOIN employees e ON e.id = s.created_by ORDER BY s.taken_for DESC`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (SnapshotInfo, error) {
		var s SnapshotInfo
		err := r.Scan(&s.TakenFor, &s.HistoryEnd, &s.Note, &s.CreatedBy, &s.CreatedAt)
		return s, err
	})
	if out == nil {
		out = []SnapshotInfo{}
	}
	return out, err
}

// GetSnapshot — сохранённый payload как есть (nil — нет такого).
func GetSnapshot(ctx context.Context, pool *pgxpool.Pool, takenFor string) (json.RawMessage, *SnapshotInfo, error) {
	var b []byte
	var s SnapshotInfo
	err := pool.QueryRow(ctx, `SELECT s.payload, to_char(s.taken_for, 'YYYY-MM-DD'), to_char(s.history_end, 'YYYY-MM-DD'),
			coalesce(s.note, ''), coalesce(e.full_name, ''), s.created_at
		FROM snapshots s LEFT JOIN employees e ON e.id = s.created_by WHERE s.taken_for = $1`, takenFor).
		Scan(&b, &s.TakenFor, &s.HistoryEnd, &s.Note, &s.CreatedBy, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	return b, &s, err
}
