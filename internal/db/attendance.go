package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/metrics"
)

// AttendanceRows — события посещаемости из Telegram (включая «upd» о приходе) с временем сообщения.
func AttendanceRows(ctx context.Context, pool *pgxpool.Pool) ([]metrics.AttRow, error) {
	rows, err := pool.Query(ctx, `SELECT a.id, e.id, e.full_name, coalesce(e.jira_login, ''), a.day, a.kind, a.from_min, a.to_min, a.minutes,
			coalesce(a.reason, ''), coalesce(a.reason_cat, ''), coalesce(a.makeup_kind, ''), coalesce(a.makeup_text, ''), a.makeup_date,
			coalesce(array_to_string(a.approved_by, ','), ''), coalesce(m.sent_at, a.created_at)
		FROM attendance_events a
		JOIN employees e ON e.id = a.employee_id
		LEFT JOIN tg_messages m ON m.chat_id = a.chat_id AND m.source = a.tg_source AND m.message_id = a.message_id
		WHERE a.source = 'telegram' AND NOT a.superseded
		ORDER BY coalesce(m.sent_at, a.created_at), a.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (metrics.AttRow, error) {
		var x metrics.AttRow
		err := r.Scan(&x.ID, &x.EmployeeID, &x.Name, &x.Login, &x.Day, &x.Kind, &x.FromMin, &x.ToMin, &x.Minutes,
			&x.Reason, &x.ReasonCat, &x.MakeupKind, &x.MakeupText, &x.MakeupDate, &x.ApprovedBy, &x.SentAt)
		return x, err
	})
}

// StaffRows — справочник для коллектива: штат, руководящий состав, уволенные.
func StaffRows(ctx context.Context, pool *pgxpool.Pool) ([]metrics.StaffRow, error) {
	rows, err := pool.Query(ctx, `SELECT full_name, coalesce(jira_login, ''), staff, management, left_at IS NOT NULL FROM employees ORDER BY full_name`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (metrics.StaffRow, error) {
		var s metrics.StaffRow
		err := r.Scan(&s.Name, &s.Login, &s.Staff, &s.Management, &s.Left)
		return s, err
	})
}
