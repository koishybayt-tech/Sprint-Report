package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/adata/reports/internal/config"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/metrics"
	"github.com/adata/reports/internal/notify"
)

// runTg: reports tg saturday [--date ГГГГ-ММ-ДД] [--send] [--force]
// Без --send только печатает текст сообщения о субботних отработках (предпросмотр); --send — отправляет в группу,
// --force — даже если на эту субботу уже отправляли.
func runTg(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 || args[0] != "saturday" {
		return errors.New("использование: reports tg saturday [--date ГГГГ-ММ-ДД] [--send] [--force]")
	}
	fs := flag.NewFlagSet("tg saturday", flag.ContinueOnError)
	date := fs.String("date", "", "суббота (по умолчанию — ближайшая)")
	send := fs.Bool("send", false, "отправить в группу (без флага — только показать текст)")
	force := fs.Bool("force", false, "отправить, даже если на эту субботу уже отправляли")
	if err := fs.Parse(args[1:]); err != nil {
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
	sat := *date
	if sat == "" {
		sat = metrics.NextSaturday(time.Now())
	}
	if !*send {
		msg, _, err := saturdayMessage(ctx, pool, sat)
		if err != nil {
			return err
		}
		fmt.Println(msg)
		return nil
	}
	return sendSaturday(ctx, cfg, pool, log, sat, *force)
}

func saturdayMessage(ctx context.Context, pool *pgxpool.Pool, sat string) (string, int, error) {
	rows, err := db.AttendanceRows(ctx, pool)
	if err != nil {
		return "", 0, err
	}
	staff, err := db.StaffRows(ctx, pool)
	if err != nil {
		return "", 0, err
	}
	rep := &metrics.SprintReport{Roles: map[string]string{}, Logins: map[string]string{}}
	att := metrics.BuildAttendance(rows, rep, nil, staff, time.Now())
	lines := metrics.SaturdayMakeups(att.Ev, sat)
	return metrics.SaturdayMessage(sat, lines), len(lines), nil
}

// sendSaturday отправляет список в группу «Adata Group» один раз на субботу (отметка — sync_runs, source = tg_saturday).
func sendSaturday(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, log *slog.Logger, sat string, force bool) error {
	if cfg.TgBotToken == "" || cfg.TgChatID == 0 {
		return errors.New("нет TG_BOT_TOKEN или TG_CHAT_ID")
	}
	if !force {
		var sent bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM sync_runs WHERE source = 'tg_saturday' AND ok AND stats->>'date' = $1)`, sat).Scan(&sent); err != nil {
			return err
		}
		if sent {
			log.Info("tg saturday: на эту субботу уже отправлено", "date", sat)
			return nil
		}
	}
	msg, n, err := saturdayMessage(ctx, pool, sat)
	if err != nil {
		return err
	}
	sendErr := notify.NewTG(cfg.TgBotToken).Send(ctx, cfg.TgChatID, msg)
	stats, _ := json.Marshal(map[string]any{"date": sat, "people": n})
	var errText *string
	if sendErr != nil {
		s := sendErr.Error()
		errText = &s
	}
	if _, err := pool.Exec(ctx, `INSERT INTO sync_runs (source, mode, finished_at, ok, stats, error) VALUES ('tg_saturday', 'send', now(), $1, $2, $3)`,
		sendErr == nil, stats, errText); err != nil && sendErr == nil {
		return err
	}
	if sendErr != nil {
		return fmt.Errorf("отправка в группу: %w", sendErr)
	}
	log.Info("tg saturday: отправлено в группу", "date", sat, "people", n)
	return nil
}

// saturdayDue — пятница, время отправки уже наступило (TG_SATURDAY_AT, по Алматы), сутки ещё не кончились.
func saturdayDue(now time.Time, at string) bool {
	hm, err := time.Parse("15:04", at)
	if err != nil {
		return false
	}
	now = now.In(metrics.Almaty)
	mark := time.Date(now.Year(), now.Month(), now.Day(), hm.Hour(), hm.Minute(), 0, 0, metrics.Almaty)
	return now.Weekday() == time.Friday && !now.Before(mark)
}
