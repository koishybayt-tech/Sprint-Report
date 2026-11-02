package tgbot

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/metrics"
	"github.com/adata/reports/internal/tgparse"
)

// Message — сообщение группы из любого источника.
type Message struct {
	ChatID    int64
	Source    string // bot | export
	MessageID int64
	FromID    int64 // Telegram user id; 0 — неизвестен
	FromName  string
	Username  string
	SentAt    time.Time
	EditedAt  *time.Time
	Text      string
	Raw       json.RawMessage
}

// Outcome — что произошло с сообщением.
type Outcome struct {
	Duplicate  bool // уже есть из другого источника
	EmployeeID int64
	How        How
	Type       tgparse.MsgType
	Status     tgparse.Status
	Events     int
}

type Store struct {
	pool     *pgxpool.Pool
	dir      *Directory
	holidays map[string]bool
}

// NewStore загружает справочник авторов и праздники.
func NewStore(ctx context.Context, pool *pgxpool.Pool) (*Store, error) {
	s := &Store{pool: pool}
	return s, s.Reload(ctx)
}

// Reload перечитывает справочник (после seed или правок алиасов).
func (s *Store) Reload(ctx context.Context) error {
	people := map[int64]*Person{}
	rows, err := s.pool.Query(ctx, `SELECT id, full_name FROM employees`)
	if err != nil {
		return err
	}
	for rows.Next() {
		p := &Person{}
		if err := rows.Scan(&p.EmployeeID, &p.FullName); err != nil {
			rows.Close()
			return err
		}
		people[p.EmployeeID] = p
	}
	rows.Close()
	rows, err = s.pool.Query(ctx, `SELECT employee_id, tg_user_id, coalesce(tg_username, ''), coalesce(tg_display, ''), coalesce(fio, '') FROM tg_aliases`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var uid *int64
		var user, disp, fio string
		if err := rows.Scan(&id, &uid, &user, &disp, &fio); err != nil {
			rows.Close()
			return err
		}
		p := people[id]
		if p == nil {
			continue
		}
		if uid != nil {
			p.UserIDs = append(p.UserIDs, *uid)
		}
		if user != "" {
			p.Usernames = append(p.Usernames, user)
		}
		if disp != "" {
			p.Displays = append(p.Displays, disp)
		}
		if fio != "" {
			p.FIOs = append(p.FIOs, fio)
		}
	}
	rows.Close()
	list := make([]Person, 0, len(people))
	for _, p := range people {
		list = append(list, *p)
	}
	s.dir = NewDirectory(list)

	s.holidays = map[string]bool{}
	rows, err = s.pool.Query(ctx, `SELECT day FROM holidays WHERE kind <> 'transfer_work'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return err
		}
		s.holidays[d.Format("2006-01-02")] = true
	}
	return rows.Err()
}

// Ingest сохраняет сообщение (raw — всегда), находит автора, разбирает и пересоздаёт события посещаемости.
// Повторная обработка того же сообщения безопасна; правка текста помечает прежние события superseded.
func (s *Store) Ingest(ctx context.Context, m Message) (Outcome, error) {
	var out Outcome
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)

	// То же сообщение уже пришло из другого источника (бот ↔ экспорт): нумерация разная, сверяем автора, время и текст.
	if m.FromID != 0 {
		var dup bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM tg_messages WHERE chat_id = $1 AND source <> $2
			AND from_id = $3 AND sent_at = $4 AND coalesce(text, '') = $5)`, m.ChatID, m.Source, m.FromID, m.SentAt, m.Text).Scan(&dup); err != nil {
			return out, err
		}
		if dup {
			out.Duplicate = true
			return out, nil
		}
	}

	var oldText *string
	err = tx.QueryRow(ctx, `SELECT text FROM tg_messages WHERE chat_id = $1 AND source = $2 AND message_id = $3`,
		m.ChatID, m.Source, m.MessageID).Scan(&oldText)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	edited := oldText != nil && *oldText != m.Text

	out.EmployeeID, out.How = s.dir.Resolve(m.FromID, m.Username, m.FromName, m.Text)
	sentLocal := m.SentAt.In(metrics.Almaty)
	res := tgparse.Parse(m.Text, sentLocal)
	out.Type, out.Status = res.Type, res.Status
	status := string(res.Status)
	if res.Type == tgparse.TypeNone {
		status = "ignored"
	}
	resJSON, _ := json.Marshal(res)
	var empID *int64
	if out.EmployeeID != 0 {
		empID = &out.EmployeeID
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tg_messages (chat_id, source, message_id, from_id, from_name, sent_at, edited_at, text, raw,
			employee_id, parse_version, parse_result, parse_status)
		VALUES ($1,$2,$3,nullif($4::bigint,0),$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (chat_id, source, message_id) DO UPDATE SET from_id = EXCLUDED.from_id, from_name = EXCLUDED.from_name,
		  edited_at = EXCLUDED.edited_at, text = EXCLUDED.text, raw = EXCLUDED.raw, employee_id = EXCLUDED.employee_id,
		  parse_version = EXCLUDED.parse_version, parse_result = EXCLUDED.parse_result, parse_status = EXCLUDED.parse_status`,
		m.ChatID, m.Source, m.MessageID, m.FromID, m.FromName, m.SentAt, m.EditedAt, m.Text, m.Raw,
		empID, tgparse.Version, resJSON, status); err != nil {
		return out, err
	}

	// Автор найден по ФИО в тексте — запоминаем его Telegram ID, дальше он узнаётся сразу.
	if out.How == ByFIO && m.FromID != 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO tg_aliases (employee_id, tg_user_id, tg_display) VALUES ($1, $2, nullif($3, ''))
			ON CONFLICT (tg_user_id) DO NOTHING`, out.EmployeeID, m.FromID, m.FromName); err != nil {
			return out, err
		}
		s.dir.byUserID[m.FromID] = out.EmployeeID
	}

	// Прежние события сообщения: при правке текста — superseded (история), иначе — пересоздаём.
	q := `DELETE FROM attendance_events`
	if edited {
		q = `UPDATE attendance_events SET superseded = true`
	}
	if _, err := tx.Exec(ctx, q+` WHERE source = 'telegram' AND NOT superseded AND chat_id = $1 AND tg_source = $2 AND message_id = $3`,
		m.ChatID, m.Source, m.MessageID); err != nil {
		return out, err
	}
	if out.EmployeeID != 0 && res.Type != tgparse.TypeNone {
		n, err := s.insertEvents(ctx, tx, m, out.EmployeeID, res, sentLocal)
		if err != nil {
			return out, err
		}
		out.Events = n
	}
	return out, tx.Commit(ctx)
}

func (s *Store) insertEvents(ctx context.Context, tx pgx.Tx, m Message, empID int64, res tgparse.Result, sent time.Time) (int, error) {
	type ev struct {
		kind, day string
		from, to  *int
		minutes   int
		unpaid    bool
	}
	var evs []ev
	if res.Type == tgparse.TypeArrived {
		at := res.ArrivedAt
		evs = append(evs, ev{kind: "arrived", day: sent.Format("2006-01-02"), from: &at})
	}
	for _, e := range res.Events {
		if e.Kind == tgparse.KindVac && s.dayOff(e.Date) {
			continue // выходные и праздники из отпуска исключаются
		}
		x := ev{kind: string(e.Kind), day: e.Date, minutes: e.Minutes, unpaid: e.Unpaid}
		if e.FromMin != 0 || e.ToMin != 0 {
			a, b := e.FromMin, e.ToMin
			x.from, x.to = &a, &b
		}
		evs = append(evs, x)
	}
	var makeupDate *string
	if res.Makeup.Date != "" {
		makeupDate = &res.Makeup.Date
	}
	for _, e := range evs {
		if _, err := tx.Exec(ctx, `INSERT INTO attendance_events (employee_id, day, kind, from_min, to_min, minutes, unpaid,
				reason, reason_cat, makeup_kind, makeup_text, makeup_date, approved_by, source, chat_id, tg_source, message_id)
			VALUES ($1,$2,$3,$4,$5,$6,$7,nullif($8,''),nullif($9,''),nullif($10,''),nullif($11,''),$12,$13,'telegram',$14,$15,$16)`,
			empID, e.day, e.kind, e.from, e.to, e.minutes, e.unpaid, res.Reason, res.ReasonCat, string(res.Makeup.Kind), res.Makeup.Text,
			makeupDate, res.ApprovedBy, m.ChatID, m.Source, m.MessageID); err != nil {
			return 0, err
		}
	}
	return len(evs), nil
}

func (s *Store) dayOff(day string) bool {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return false
	}
	return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday || s.holidays[day]
}
