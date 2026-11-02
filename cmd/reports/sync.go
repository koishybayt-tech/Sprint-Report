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
)

// runSync: reports sync jira [--full] | reports sync bitrix.
// Для Jira пока есть только полная синхронизация; инкрементальная — следующий этап.
func runSync(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 {
		return errors.New("укажите источник: reports sync jira [--full] | bitrix")
	}
	source, args := args[0], args[1:]
	switch source {
	case "jira":
	case "bitrix":
		return runSyncBitrix(ctx, log)
	default:
		return fmt.Errorf("источник %q пока не поддерживается", source)
	}
	fs := flag.NewFlagSet("sync jira", flag.ContinueOnError)
	full := fs.Bool("full", false, "полная синхронизация открытых спринтов")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !*full {
		log.Info("sync jira: инкрементальная синхронизация ещё не реализована, выполняю полную")
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
	return syncJira(ctx, log, cfg, client, pool)
}

// syncJira — один полный проход синхронизации Jira; общий для команды sync и worker.
func syncJira(ctx context.Context, log *slog.Logger, cfg config.Config, client *jira.Client, pool *pgxpool.Pool) error {
	me, err := client.Myself(ctx)
	if err != nil {
		return fmt.Errorf("проверка токена Jira: %w", err)
	}
	log.Info("sync jira: старт", "base", cfg.JiraBaseURL, "user", me.Name)
	t0 := time.Now()
	st, err := jira.FullSync(ctx, pool, client, log)
	if err != nil {
		return err
	}
	log.Info("sync jira: готово", "issues", st.Issues, "future_issues", st.FutureIssues, "projects", st.Projects,
		"sprints", st.Sprints, "future_sprints", st.FutureSprints, "team_leads", st.TeamLeads,
		"board_errors", st.BoardErrors, "took", time.Since(t0).Round(time.Second).String())
	return nil
}
