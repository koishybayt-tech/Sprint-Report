package tgbot

import (
	"context"
	"errors"
	"strings"
	"time"
)

// UnknownAuthor — аккаунт Telegram, чьи сообщения про посещаемость не удалось связать с сотрудником (очередь «Кто это?»).
type UnknownAuthor struct {
	FromID   *int64    `json:"from_id"` // nil — известно только имя (HTML-экспорт)
	FromName string    `json:"from_name"`
	Messages int       `json:"messages"` // сообщений про посещаемость
	LastAt   time.Time `json:"last_at"`
	Sample   string    `json:"sample"` // первая строка последнего такого сообщения — обычно ФИО
}

// UnknownAuthors — авторы без сотрудника, у которых есть хотя бы одно сообщение про посещаемость
// (команды и обычная переписка в очередь не попадают).
func (s *Store) UnknownAuthors(ctx context.Context) ([]UnknownAuthor, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT from_id, coalesce(from_name, ''), count(*), max(sent_at),
		       (array_agg(text ORDER BY sent_at DESC))[1]
		FROM tg_messages
		WHERE employee_id IS NULL AND parse_status IS DISTINCT FROM 'ignored'
		GROUP BY from_id, coalesce(from_name, '')
		ORDER BY count(*) DESC, max(sent_at) DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UnknownAuthor{}
	for rows.Next() {
		var a UnknownAuthor
		var text *string
		if err := rows.Scan(&a.FromID, &a.FromName, &a.Messages, &a.LastAt, &text); err != nil {
			return nil, err
		}
		if text != nil {
			a.Sample, _, _ = strings.Cut(strings.TrimSpace(*text), "\n")
			if r := []rune(a.Sample); len(r) > 80 {
				a.Sample = string(r[:80]) + "…"
			}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// JiraUser — сотрудник с логином Jira.
type JiraUser struct {
	ID        int64  `json:"id"`
	FullName  string `json:"full_name"`
	JiraLogin string `json:"jira_login"`
	Active    bool   `json:"active"` // активен в Jira; неактивных показываем в конце — в истории группы есть и ушедшие
}

// UnmappedJiraUsers — сотрудники с логином Jira, ещё не связанные ни с одним аккаунтом Telegram; сначала активные.
func (s *Store) UnmappedJiraUsers(ctx context.Context) ([]JiraUser, error) {
	rows, err := s.pool.Query(ctx, `SELECT e.id, e.full_name, e.jira_login, e.jira_active FROM employees e
		WHERE e.jira_login IS NOT NULL AND e.left_at IS NULL
		  AND NOT EXISTS (SELECT 1 FROM tg_aliases a WHERE a.employee_id = e.id)
		ORDER BY e.jira_active DESC, e.full_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []JiraUser{}
	for rows.Next() {
		var u JiraUser
		if err := rows.Scan(&u.ID, &u.FullName, &u.JiraLogin, &u.Active); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

var ErrAliasConflict = errors.New("этот аккаунт Telegram уже связан с другим сотрудником")

// MapAuthor связывает аккаунт Telegram с сотрудником и заново разбирает все его сообщения без автора.
// Возвращает, сколько сообщений и событий получилось.
func (s *Store) MapAuthor(ctx context.Context, fromID *int64, fromName string, employeeID int64) (messages, events int, err error) {
	fromName = strings.TrimSpace(fromName)
	if fromName == "" && fromID == nil {
		return 0, 0, errors.New("не указан аккаунт Telegram")
	}
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM employees WHERE id = $1 AND jira_login IS NOT NULL)`, employeeID).Scan(&ok); err != nil {
		return 0, 0, err
	}
	if !ok {
		return 0, 0, errors.New("сотрудник с логином Jira не найден")
	}
	if fromID != nil {
		var other int64
		err := s.pool.QueryRow(ctx, `SELECT employee_id FROM tg_aliases WHERE tg_user_id = $1`, *fromID).Scan(&other)
		if err == nil && other != employeeID {
			return 0, 0, ErrAliasConflict
		}
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO tg_aliases (employee_id, tg_user_id, tg_display) VALUES ($1, $2, nullif($3, ''))
		ON CONFLICT (tg_user_id) DO NOTHING`, employeeID, fromID, fromName); err != nil {
		return 0, 0, err
	}
	if err := s.Reload(ctx); err != nil {
		return 0, 0, err
	}
	// Привязка вручную надёжнее эвристик: сообщения этого аккаунта отдаём выбранному сотруднику напрямую.
	if fromID != nil {
		s.dir.byUserID[*fromID] = employeeID
	} else {
		s.dir.byDisplay[normDisplay(fromName)] = []int64{employeeID}
	}

	rows, err := s.pool.Query(ctx, `SELECT chat_id, source, message_id, coalesce(from_id, 0), coalesce(from_name, ''), sent_at, edited_at, coalesce(text, ''), raw
		FROM tg_messages WHERE employee_id IS NULL AND (($1::bigint IS NOT NULL AND from_id = $1) OR (from_id IS NULL AND from_name = $2))
		ORDER BY sent_at`, fromID, fromName)
	if err != nil {
		return 0, 0, err
	}
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ChatID, &m.Source, &m.MessageID, &m.FromID, &m.FromName, &m.SentAt, &m.EditedAt, &m.Text, &m.Raw); err != nil {
			rows.Close()
			return 0, 0, err
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	for _, m := range msgs {
		out, err := s.Ingest(ctx, m)
		if err != nil {
			return messages, events, err
		}
		messages++
		events += out.Events
	}
	return messages, events, nil
}

// ReprocessUnknown заново ищет автора у сообщений про посещаемость, которые пока ни с кем не связаны
// (после новых правил сравнения имён или привязок). Возвращает, у скольких автор нашёлся.
func (s *Store) ReprocessUnknown(ctx context.Context) (int, error) {
	rows, err := s.pool.Query(ctx, `SELECT chat_id, source, message_id, coalesce(from_id, 0), coalesce(from_name, ''), sent_at, edited_at, coalesce(text, ''), raw
		FROM tg_messages WHERE employee_id IS NULL AND parse_status IS DISTINCT FROM 'ignored' ORDER BY sent_at`)
	if err != nil {
		return 0, err
	}
	var msgs []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ChatID, &m.Source, &m.MessageID, &m.FromID, &m.FromName, &m.SentAt, &m.EditedAt, &m.Text, &m.Raw); err != nil {
			rows.Close()
			return 0, err
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	found := 0
	for _, m := range msgs {
		if id, _ := s.dir.Resolve(m.FromID, m.Username, m.FromName, m.Text); id == 0 {
			continue // всё ещё неизвестен — не трогаем
		}
		out, err := s.Ingest(ctx, m)
		if err != nil {
			return found, err
		}
		if out.EmployeeID != 0 {
			found++
		}
	}
	return found, nil
}
