package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/bitrix"
	"github.com/adata/reports/internal/config"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/jira"
)

// runBitrix: reports bitrix probe --id <табельный ID> — печатает сырой ответ API отпусков (ключ замаскирован)
// и то, как его понял разбор. Нужен, чтобы подогнать bitrix.Parse под настоящий формат.
func runBitrix(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "probe" {
		return errors.New("использование: reports bitrix probe --id <табельный ID>")
	}
	fs := flag.NewFlagSet("bitrix probe", flag.ContinueOnError)
	id := fs.String("id", "", "табельный ID сотрудника")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *id == "" {
		return errors.New("укажите --id <табельный ID>")
	}
	cfg, err := loadBitrixConfig()
	if err != nil {
		return err
	}
	c, err := bitrix.New(cfg.BitrixVacationURL, cfg.BitrixAPIKey)
	if err != nil {
		return err
	}
	resp, err := c.Fetch(ctx, *id)
	fmt.Printf("GET %s?id=%s&api_key=***\n", cfg.BitrixVacationURL, *id)
	if resp.Status != 0 {
		fmt.Printf("HTTP %d  Content-Type: %s\n\n", resp.Status, resp.ContentType)
		var pretty bytes.Buffer
		if json.Indent(&pretty, resp.Body, "", "  ") == nil {
			fmt.Println(pretty.String())
		} else {
			fmt.Println(string(resp.Body))
		}
	}
	if err != nil {
		return err
	}
	vac, skipped, _, perr := bitrix.Parse(resp.Body)
	fmt.Printf("\nразбор: учтено %d, пропущено %d", len(vac), skipped)
	if perr != nil {
		fmt.Printf(", ошибка: %s", c.Mask(perr.Error()))
	}
	fmt.Println()
	for _, v := range vac {
		fmt.Printf("  %s  %s – %s  %s (%s) id=%s\n", v.Kind, v.From.Format("2006-01-02"), v.To.Format("2006-01-02"), v.Type, v.Status, v.ID)
	}
	return nil
}

// loadBitrixConfig — probe не нужна база, поэтому DATABASE_URL не обязателен.
func loadBitrixConfig() (config.Config, error) {
	if os.Getenv("DATABASE_URL") == "" {
		os.Setenv("DATABASE_URL", "unused")
	}
	return config.Load()
}

// syncBitrix — один проход синхронизации отпусков; общий для `sync bitrix` и worker.
func syncBitrix(ctx context.Context, log *slog.Logger, c *bitrix.Client, jc *jira.Client, pool *pgxpool.Pool) error {
	log.Info("sync bitrix: старт")
	t0 := time.Now()
	st, err := bitrix.Sync(ctx, pool, c, jc, log)
	if err != nil {
		return errors.New(c.Mask(err.Error()))
	}
	log.Info("sync bitrix: готово", "employees", st.Employees, "no_tab_id", st.NoTabID, "emails_found", st.EmailsFound,
		"absences", st.Absences, "skipped", st.Skipped, "no_vacations", st.NoVacations, "errors", st.Errors, "seen", st.Seen, "took", time.Since(t0).Round(time.Second).String())
	return nil
}

func runSyncBitrix(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	c, err := bitrix.New(cfg.BitrixVacationURL, cfg.BitrixAPIKey)
	if err != nil {
		return err
	}
	jc, err := jira.New(cfg.JiraBaseURL, cfg.JiraPAT)
	if err != nil {
		log.Warn("sync bitrix: без Jira email не подтянуть, используются tab_id из seed и уже сохранённые email", "reason", err.Error())
		jc = nil
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	return syncBitrix(ctx, log, c, jc, pool)
}
