package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LeadProject — строка справочника «руководитель → проект».
type LeadProject struct {
	Lead       string `json:"lead"`
	ProjectKey string `json:"project_key"`
	Title      string `json:"title"`
	Position   int    `json:"position"`
}

func ListLeadProjects(ctx context.Context, pool *pgxpool.Pool) ([]LeadProject, error) {
	rows, err := pool.Query(ctx, `SELECT lead, project_key, title, position FROM lead_projects ORDER BY lead, position, project_key`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (LeadProject, error) {
		var p LeadProject
		err := r.Scan(&p.Lead, &p.ProjectKey, &p.Title, &p.Position)
		return p, err
	})
	if out == nil {
		out = []LeadProject{}
	}
	return out, err
}

// SaveLeadProject добавляет пару или обновляет название и порядок существующей.
func SaveLeadProject(ctx context.Context, pool *pgxpool.Pool, p LeadProject) error {
	_, err := pool.Exec(ctx, `INSERT INTO lead_projects (lead, project_key, title, position) VALUES ($1, $2, $3, $4)
		ON CONFLICT (lead, project_key) DO UPDATE SET title = EXCLUDED.title, position = EXCLUDED.position`,
		p.Lead, p.ProjectKey, p.Title, p.Position)
	return err
}

var ErrNotFound = errors.New("не найдено")

func DeleteLeadProject(ctx context.Context, pool *pgxpool.Pool, lead, project string) error {
	tag, err := pool.Exec(ctx, `DELETE FROM lead_projects WHERE lead = $1 AND project_key = $2`, lead, project)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if err == nil {
		// Расчёт по руководителю без проектов больше не нужен; остальным его пересчитает worker.
		_, err = pool.Exec(ctx, `DELETE FROM lead_control WHERE lead = $1 AND NOT EXISTS (SELECT 1 FROM lead_projects WHERE lead = $1)`, lead)
	}
	return err
}

// SaveLeadControl — готовый расчёт по руководителю (пишет worker после синхронизации Jira).
func SaveLeadControl(ctx context.Context, pool *pgxpool.Pool, lead string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, `INSERT INTO lead_control (lead, payload, computed_at) VALUES ($1, $2, now())
		ON CONFLICT (lead) DO UPDATE SET payload = EXCLUDED.payload, computed_at = now()`, lead, b)
	return err
}

// LeadControl — сохранённый расчёт (nil — ещё не считался).
func LeadControl(ctx context.Context, pool *pgxpool.Pool, lead string) (json.RawMessage, *time.Time, error) {
	var b []byte
	var at time.Time
	err := pool.QueryRow(ctx, `SELECT payload, computed_at FROM lead_control WHERE lead = $1`, lead).Scan(&b, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	return b, &at, err
}
