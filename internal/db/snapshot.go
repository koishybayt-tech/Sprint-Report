package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
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

// DeleteSnapshot удаляет снепшот на дату (ErrNotFound — такого нет).
func DeleteSnapshot(ctx context.Context, pool *pgxpool.Pool, takenFor string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM snapshots WHERE taken_for = $1`, takenFor)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
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
