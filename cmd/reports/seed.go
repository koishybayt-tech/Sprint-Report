package main

import (
	"context"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/adata/reports/internal/config"
	"github.com/adata/reports/internal/db"
)

// runSeed загружает справочники из CSV. Повторный запуск безопасен: записи обновляются, а не дублируются.
func runSeed(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("seed", flag.ContinueOnError)
	dir := fs.String("dir", "seed", "каталог с employees.csv, tg_aliases.csv, holidays_kz.csv")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	steps := []struct {
		file string
		fn   func(context.Context, pgx.Tx, []map[string]string) (int, int, error)
	}{
		{"employees.csv", seedEmployees},
		{"tg_aliases.csv", seedAliases},
		{"holidays_kz.csv", seedHolidays},
	}
	for _, s := range steps {
		rows, err := readCSV(filepath.Join(*dir, s.file))
		if err != nil {
			return err
		}
		n, skipped, err := s.fn(ctx, tx, rows)
		if err != nil {
			return fmt.Errorf("%s: %w", s.file, err)
		}
		log.Info("seed", "file", s.file, "rows", len(rows), "written", n, "skipped", skipped)
	}
	return tx.Commit(ctx)
}

// employees.csv: full_name,jira_login,position,team_lead,project_lead,staff,left,tab_id.
// tab_id — табельный ID для API отпусков, если он не совпадает с email в Jira (пусто — берётся из email).
// team_lead/project_lead в CSV — флаги 0/1, а в схеме это текст (имя руководителя), поэтому пока не загружаются.
// Если jira_login уже занят другим ФИО, строка пропускается с предупреждением — дубль надо разрешить в CSV.
func seedEmployees(ctx context.Context, tx pgx.Tx, rows []map[string]string) (int, int, error) {
	n, skipped := 0, 0
	for _, r := range rows {
		if login := r["jira_login"]; login != "" {
			var other string
			err := tx.QueryRow(ctx, `SELECT full_name FROM employees WHERE jira_login = $1 AND full_name <> $2`,
				login, r["full_name"]).Scan(&other)
			if err == nil {
				slog.Warn("seed: jira_login уже у другого сотрудника, строка пропущена",
					"jira_login", login, "full_name", r["full_name"], "taken_by", other)
				skipped++
				continue
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return n, skipped, err
			}
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO employees (full_name, jira_login, position, staff, tab_id)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (full_name) DO UPDATE
			SET jira_login = EXCLUDED.jira_login, position = EXCLUDED.position,
			    staff = EXCLUDED.staff, tab_id = EXCLUDED.tab_id, updated_at = now()`,
			r["full_name"], nullable(r["jira_login"]), nullable(r["position"]), r["staff"] == "1", nullable(r["tab_id"]))
		if err != nil {
			return n, skipped, fmt.Errorf("%s: %w", r["full_name"], err)
		}
		n++
	}
	return n, skipped, nil
}

// tg_aliases.csv: tg_display_name,full_name,jira_name,jira_login,source.
// full_name — как человек подписывается в сообщениях (сохраняется в fio). Сотрудник ищется по jira_login,
// а без него — по jira_name или full_name среди ФИО сотрудников. Ненайденные строки пропускаются.
func seedAliases(ctx context.Context, tx pgx.Tx, rows []map[string]string) (int, int, error) {
	n, skipped := 0, 0
	for _, r := range rows {
		var empID int64
		err := tx.QueryRow(ctx, `SELECT id FROM employees
			WHERE ($1 <> '' AND jira_login = $1) OR ($1 = '' AND full_name IN ($2, $3))
			ORDER BY (jira_login = $1) DESC LIMIT 1`, r["jira_login"], r["jira_name"], r["full_name"]).Scan(&empID)
		if errors.Is(err, pgx.ErrNoRows) {
			skipped++ // «не определён», «нет в Jira» или сотрудник не найден
			continue
		}
		if err != nil {
			return n, skipped, fmt.Errorf("%s: %w", r["tg_display_name"], err)
		}
		tag, err := tx.Exec(ctx, `UPDATE tg_aliases SET fio = nullif($3, '') WHERE employee_id = $1 AND tg_display = $2`,
			empID, r["tg_display_name"], r["full_name"])
		if err != nil {
			return n, skipped, err
		}
		if tag.RowsAffected() == 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO tg_aliases (employee_id, tg_display, fio) VALUES ($1, $2, nullif($3, ''))`,
				empID, r["tg_display_name"], r["full_name"]); err != nil {
				return n, skipped, fmt.Errorf("%s: %w", r["tg_display_name"], err)
			}
		}
		n++
	}
	return n, skipped, nil
}

// holidays_kz.csv: date,name. Тип дня (перенос и т.п.) пока не размечается — всё 'holiday'.
func seedHolidays(ctx context.Context, tx pgx.Tx, rows []map[string]string) (int, int, error) {
	n := 0
	for _, r := range rows {
		_, err := tx.Exec(ctx, `
			INSERT INTO holidays (day, name) VALUES ($1, $2)
			ON CONFLICT (day) DO UPDATE SET name = EXCLUDED.name`,
			r["date"], r["name"])
		if err != nil {
			return n, 0, fmt.Errorf("%s: %w", r["date"], err)
		}
		n++
	}
	return n, 0, nil
}

// readCSV читает CSV с заголовком в срез map[колонка]значение.
func readCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(recs) == 0 {
		return nil, nil
	}
	header := recs[0]
	header[0] = strings.TrimPrefix(header[0], "\uFEFF") // BOM
	rows := make([]map[string]string, 0, len(recs)-1)
	for _, rec := range recs[1:] {
		m := make(map[string]string, len(header))
		for i, h := range header {
			if i < len(rec) {
				m[h] = strings.TrimSpace(rec[i])
			}
		}
		rows = append(rows, m)
	}
	return rows, nil
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
