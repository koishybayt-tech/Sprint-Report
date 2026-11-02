package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/config"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/jira"
	"github.com/adata/reports/internal/metrics"
)

// runSync: reports sync jira [--full] | reports sync jira-history [--full] | reports sync bitrix.
// Без --full Jira синхронизируется инкрементально (только изменённые задачи); --full — всё, плюс проекты, тимлиды, справочник.
// jira-history — задачи года с историей статуса и исполнителя для карточки сотрудника.
func runSync(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		return errors.New("укажите источник: reports sync jira [--full] | jira-history [--full] | bitrix")
	}
	source, args := args[0], args[1:]
	switch source {
	case "jira", "jira-history":
	case "bitrix":
		return runSyncBitrix(ctx, log)
	default:
		return fmt.Errorf("источник %q пока не поддерживается", source)
	}
	fs := flag.NewFlagSet("sync jira", flag.ContinueOnError)
	full := fs.Bool("full", false, "полная синхронизация вместо инкрементальной")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client, err := jira.New(cfg.JiraBaseURL, cfg.JiraPAT)
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if source == "jira-history" {
		return syncJiraHistory(ctx, log, client, pool, *full)
	}
	return syncJira(ctx, log, cfg, client, pool, *full)
}

// syncJiraHistory — проход истории задач года; без прошлой успешной загрузки делает полную.
// Если полная уже идёт (в другом процессе) — тихо пропускает.
func syncJiraHistory(ctx context.Context, log *slog.Logger, client *jira.Client, pool *pgxpool.Pool, full bool) error {
	t0 := time.Now()
	st, err := jira.HistorySync(ctx, pool, client, log, full)
	if errors.Is(err, jira.ErrNoBaseline) {
		log.Info("sync jira history: загрузок ещё не было — полная за год, это ~20 минут")
		full = true
		st, err = jira.HistorySync(ctx, pool, client, log, true)
	}
	if errors.Is(err, jira.ErrHistoryRunning) {
		log.Info("sync jira history: пропуск — полная загрузка уже идёт")
		return nil
	}
	if err != nil {
		return err
	}
	log.Info("sync jira history: готово", "full", full, "issues", st.Issues, "changes", st.Changes, "refetch", st.Refetch,
		"window", st.Since, "took", time.Since(t0).Round(time.Second).String())
	return nil
}

// syncJira — один проход синхронизации Jira: полный (full) или инкрементальный; общий для команды sync и worker.
// Инкрементальный без прошлых запусков сам становится полным.
func syncJira(ctx context.Context, log *slog.Logger, cfg config.Config, client *jira.Client, pool *pgxpool.Pool, full bool) error {
	me, err := client.Myself(ctx)
	if err != nil {
		return fmt.Errorf("проверка токена Jira: %w", err)
	}
	t0 := time.Now()
	if !full {
		log.Info("sync jira: старт (изменённые задачи)", "user", me.Name)
		st, err := jira.IncrementalSync(ctx, pool, client, log)
		switch {
		case errors.Is(err, jira.ErrNoBaseline):
			log.Info("sync jira: прошлых запусков нет — делаю полную")
		case err != nil:
			return err
		default:
			log.Info("sync jira: готово (изменённые)", "changed", st.Changed, "added", st.Added, "removed", st.Removed,
				"open", st.Open, "future", st.Future, "window", st.Since, "board_errors", st.BoardErrors, "took", time.Since(t0).Round(time.Second).String())
			return nil
		}
	}
	log.Info("sync jira: старт (полная)", "base", cfg.JiraBaseURL, "user", me.Name)
	st, err := jira.FullSync(ctx, pool, client, log)
	if err != nil {
		return err
	}
	log.Info("sync jira: готово (полная)", "issues", st.Issues, "future_issues", st.FutureIssues, "projects", st.Projects,
		"sprints", st.Sprints, "future_sprints", st.FutureSprints, "team_leads", st.TeamLeads, "new_employees", st.NewEmployees,
		"board_errors", st.BoardErrors, "took", time.Since(t0).Round(time.Second).String())
	return nil
}

// fullSyncDue: полная синхронизация нужна, если её не было после последнего наступления JIRA_FULL_SYNC_AT
// (по Алматы). Перезапуск worker днём не вызывает лишнюю полную, а ночная случится на ближайшем тике.
func fullSyncDue(ctx context.Context, pool *pgxpool.Pool, at string) (bool, error) {
	var last *time.Time
	if err := pool.QueryRow(ctx, `SELECT max(started_at) FROM sync_runs WHERE source = 'jira' AND mode = 'full' AND ok`).Scan(&last); err != nil {
		return false, err
	}
	if last == nil {
		return true, nil
	}
	hm, err := time.Parse("15:04", at)
	if err != nil {
		return false, err
	}
	now := time.Now().In(metrics.Almaty)
	mark := time.Date(now.Year(), now.Month(), now.Day(), hm.Hour(), hm.Minute(), 0, 0, metrics.Almaty)
	if now.Before(mark) {
		mark = mark.AddDate(0, 0, -1)
	}
	return last.Before(mark), nil
}
