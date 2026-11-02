package bitrix

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/jira"
)

type SyncStats struct {
	Employees   int  `json:"employees"`    // с табельным ID
	NoTabID     int  `json:"no_tab_id"`    // с логином Jira, но без табельного ID
	EmailsFound int  `json:"emails_found"` // email подтянут из Jira в этот раз
	Absences    int  `json:"absences"`     // записано отпусков
	Skipped     int  `json:"skipped"`      // записи API другого типа, «Прерван» или без дат
	NoVacations int  `json:"no_vacations"` // API ответил 404: заявлений нет
	Errors      int  `json:"errors"`       // сотрудники, по которым запрос не удался
	Seen        Seen `json:"seen"`         // все встретившиеся типы и статусы заявлений
}

var digitsRe = regexp.MustCompile(`^\d+$`)

// Sync тянет отпуска по всем сотрудникам с табельным ID и пишет их в absences (source='bitrix').
// jc нужен, чтобы взять email из Jira у сотрудников, у которых его ещё нет; может быть nil.
func Sync(ctx context.Context, pool *pgxpool.Pool, c *Client, jc *jira.Client, log *slog.Logger) (SyncStats, error) {
	st := SyncStats{Seen: Seen{}}
	var runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO sync_runs (source, mode) VALUES ('bitrix', 'full') RETURNING id`).Scan(&runID); err != nil {
		return st, err
	}
	err := sync(ctx, pool, c, jc, log, &st)
	stats, _ := json.Marshal(st)
	var errText *string
	if err != nil {
		s := c.Mask(err.Error())
		errText = &s
	}
	if _, e := pool.Exec(context.WithoutCancel(ctx),
		`UPDATE sync_runs SET finished_at = now(), ok = $2, stats = $3, error = $4 WHERE id = $1`,
		runID, err == nil, stats, errText); e != nil && err == nil {
		err = e
	}
	return st, err
}

func sync(ctx context.Context, pool *pgxpool.Pool, c *Client, jc *jira.Client, log *slog.Logger, st *SyncStats) error {
	// 1. Email из Jira для тех, у кого его ещё нет: табельный ID — часть email до «@».
	if jc != nil {
		rows, err := pool.Query(ctx, `SELECT id, jira_login FROM employees
			WHERE jira_login IS NOT NULL AND jira_email IS NULL AND left_at IS NULL`)
		if err != nil {
			return err
		}
		type emp struct {
			id    int64
			login string
		}
		var need []emp
		for rows.Next() {
			var e emp
			if err := rows.Scan(&e.id, &e.login); err != nil {
				rows.Close()
				return err
			}
			need = append(need, e)
		}
		rows.Close()
		for _, e := range need {
			u, err := jc.UserByLogin(ctx, e.login)
			if err != nil {
				log.Warn("bitrix: email из Jira", "login", e.login, "err", err)
				continue
			}
			if u.EmailAddress == "" {
				continue
			}
			if _, err := pool.Exec(ctx, `UPDATE employees SET jira_email = $2, updated_at = now() WHERE id = $1`, e.id, u.EmailAddress); err != nil {
				return err
			}
			st.EmailsFound++
		}
	}

	// 2. Сотрудники с табельным ID: tab_id из seed, иначе цифровая часть email до «@».
	rows, err := pool.Query(ctx, `SELECT id, full_name, coalesce(jira_login, ''), coalesce(tab_id, ''), coalesce(jira_email, '')
		FROM employees WHERE left_at IS NULL ORDER BY full_name`)
	if err != nil {
		return err
	}
	type emp struct {
		id                        int64
		name, login, tabID, email string
	}
	var emps []emp
	var noTab []string
	for rows.Next() {
		var e emp
		if err := rows.Scan(&e.id, &e.name, &e.login, &e.tabID, &e.email); err != nil {
			rows.Close()
			return err
		}
		if e.tabID == "" {
			if local, _, ok := strings.Cut(e.email, "@"); ok && digitsRe.MatchString(local) {
				e.tabID = local
			}
		}
		if e.tabID == "" {
			if e.login != "" {
				noTab = append(noTab, e.login)
			}
			continue
		}
		emps = append(emps, e)
	}
	rows.Close()
	st.Employees, st.NoTabID = len(emps), len(noTab)
	if len(noTab) > 0 {
		log.Warn("bitrix: нет табельного ID — заполните tab_id в seed/employees.csv", "logins", strings.Join(noTab, ", "))
	}

	// 3. Отпуска по каждому. Ошибка по одному сотруднику не валит синк; его прежние записи не трогаем.
	for _, e := range emps {
		resp, err := c.Fetch(ctx, e.tabID)
		var vac []Vacation
		var skipped int
		var seen Seen
		switch {
		case NoVacations(err):
			st.NoVacations++ // заявлений нет — прежние записи сотрудника из Bitrix удаляются
		case err != nil:
			st.Errors++
			log.Warn("bitrix: запрос отпусков", "employee", e.name, "err", err.Error())
			continue
		default:
			if vac, skipped, seen, err = Parse(resp.Body); err != nil {
				st.Errors++
				log.Warn("bitrix: разбор ответа", "employee", e.name, "err", c.Mask(err.Error()))
				continue
			}
		}
		st.Skipped += skipped
		for t, byStatus := range seen {
			if st.Seen[t] == nil {
				st.Seen[t] = map[string]int{}
			}
			for s, n := range byStatus {
				st.Seen[t][s] += n
			}
		}
		if err := saveEmployee(ctx, pool, e.id, e.tabID, vac); err != nil {
			return fmt.Errorf("%s: %w", e.name, err)
		}
		st.Absences += len(vac)
	}
	return nil
}

// saveEmployee заменяет отпуска сотрудника из Bitrix свежим списком: записи, которых больше нет в API, удаляются.
func saveEmployee(ctx context.Context, pool *pgxpool.Pool, empID int64, tabID string, vac []Vacation) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ids := make([]string, 0, len(vac))
	for _, v := range vac {
		ext := externalID(tabID, v)
		ids = append(ids, ext)
		if _, err := tx.Exec(ctx, `INSERT INTO absences (employee_id, kind, date_from, date_to, source, external_id, note)
			VALUES ($1, $2, $3, $4, 'bitrix', $5, $6)
			ON CONFLICT (source, external_id) DO UPDATE SET employee_id = EXCLUDED.employee_id, kind = EXCLUDED.kind,
			  date_from = EXCLUDED.date_from, date_to = EXCLUDED.date_to, note = EXCLUDED.note`,
			empID, v.Kind, v.From, v.To, ext, v.Type); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM absences WHERE source = 'bitrix' AND employee_id = $1 AND NOT (external_id = ANY($2))`,
		empID, ids); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// externalID — id записи Bitrix, а если API его не отдаёт — (табельный ID, тип, даты).
func externalID(tabID string, v Vacation) string {
	if v.ID != "" {
		return "bx:" + v.ID
	}
	return fmt.Sprintf("%s:%s:%s:%s", tabID, v.Kind, v.From.Format("2006-01-02"), v.To.Format("2006-01-02"))
}
