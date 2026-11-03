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
	UserID     *int64 `json:"user_id"`   // пользователь сервиса, которому назначен проект; nil — руководитель только по имени из Jira
	UserName   string `json:"user_name"` // ФИО пользователя для показа; при записи игнорируется
}

func ListLeadProjects(ctx context.Context, pool *pgxpool.Pool) ([]LeadProject, error) {
	rows, err := pool.Query(ctx, `SELECT lp.lead, lp.project_key, lp.title, lp.position, lp.user_id, coalesce(u.full_name, '')
		FROM lead_projects lp LEFT JOIN users u ON u.id = lp.user_id ORDER BY lp.lead, lp.position, lp.project_key`)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (LeadProject, error) {
		var p LeadProject
		err := r.Scan(&p.Lead, &p.ProjectKey, &p.Title, &p.Position, &p.UserID, &p.UserName)
		return p, err
	})
	if out == nil {
		out = []LeadProject{}
	}
	return out, err
}

// SaveLeadProject добавляет пару или обновляет название, порядок и пользователя существующей.
func SaveLeadProject(ctx context.Context, pool *pgxpool.Pool, p LeadProject) error {
	_, err := pool.Exec(ctx, `INSERT INTO lead_projects (lead, project_key, title, position, user_id) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (lead, project_key) DO UPDATE SET title = EXCLUDED.title, position = EXCLUDED.position, user_id = EXCLUDED.user_id`,
		p.Lead, p.ProjectKey, p.Title, p.Position, p.UserID)
	return err
}

var ErrNotFound = errors.New("не найдено")

// UserLeadLabel — под каким именем пользователь значится руководителем: ФИО связанного сотрудника (как в Jira),
// иначе его собственное ФИО. Только активные пользователи; иначе ErrNotFound.
func UserLeadLabel(ctx context.Context, pool *pgxpool.Pool, userID int64) (string, error) {
	var n string
	err := pool.QueryRow(ctx, `SELECT coalesce(e.full_name, u.full_name) FROM users u LEFT JOIN employees e ON e.id = u.employee_id
		WHERE u.id = $1 AND u.active`, userID).Scan(&n)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return n, err
}

// UserLeads — руководители, под которыми пользователю назначены проекты.
func UserLeads(ctx context.Context, pool *pgxpool.Pool, userID int64) ([]string, error) {
	rows, err := pool.Query(ctx, `SELECT DISTINCT lead FROM lead_projects WHERE user_id = $1 ORDER BY lead`, userID)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if out == nil {
		out = []string{}
	}
	return out, err
}

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
