package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adata/reports/internal/config"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/tgbot"
)

// runImportTelegram: reports import-telegram --file result.json | <папка HTML-экспорта> — история группы из Telegram Desktop.
// Идемпотентно: повторный импорт того же файла и сообщения, уже полученные ботом, не дублируются.
func runImportTelegram(ctx context.Context, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("import-telegram", flag.ContinueOnError)
	file := fs.String("file", "", "result.json (JSON-экспорт) или папка с messages.html (HTML-экспорт) Telegram Desktop")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *file == "" {
		return errors.New("укажите --file result.json")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var exp tgbot.Export
	if st, err := os.Stat(*file); err != nil {
		return err
	} else if st.IsDir() || strings.HasSuffix(strings.ToLower(*file), ".html") {
		// HTML-экспорт (формат по умолчанию): id чата в нём нет — берём TG_CHAT_ID.
		if cfg.TgChatID == 0 {
			return errors.New("для HTML-экспорта нужен TG_CHAT_ID в .env")
		}
		dir := *file
		if !st.IsDir() {
			dir = filepath.Dir(*file)
		}
		if exp, err = tgbot.ParseExportDir(dir, cfg.TgChatID); err != nil {
			return err
		}
		log.Info("import-telegram: HTML-экспорт", "chat", exp.Name, "chat_id", cfg.TgChatID, "messages", len(exp.Messages))
	} else {
		f, err := os.Open(*file)
		if err != nil {
			return err
		}
		exp, err = tgbot.ParseExport(f)
		f.Close()
		if err != nil {
			return err
		}
		if cfg.TgChatID != 0 && exp.ChatID != cfg.TgChatID {
			return fmt.Errorf("экспорт чата %q (id %d), а TG_CHAT_ID = %d — это другой чат?", exp.Name, exp.ChatID, cfg.TgChatID)
		}
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	store, err := tgbot.NewStore(ctx, pool)
	if err != nil {
		return err
	}

	var runID int64
	if err := pool.QueryRow(ctx, `INSERT INTO sync_runs (source, mode) VALUES ('telegram', 'import') RETURNING id`).Scan(&runID); err != nil {
		return err
	}
	t0 := time.Now()
	st := struct {
		Messages   int            `json:"messages"`
		Duplicates int            `json:"duplicates"`
		Events     int            `json:"events"`
		Manual     int            `json:"manual"`
		Types      map[string]int `json:"types"`
		Authors    map[string]int `json:"authors"` // как найден автор
	}{Types: map[string]int{}, Authors: map[string]int{}}
	unknown := map[string]int{} // имя аккаунта → сообщений про посещаемость без автора
	var ierr error
	for _, m := range exp.Messages {
		out, err := store.Ingest(ctx, m)
		if err != nil {
			ierr = fmt.Errorf("сообщение %d: %w", m.MessageID, err)
			break
		}
		st.Messages++
		if out.Duplicate {
			st.Duplicates++
			continue
		}
		st.Types[string(out.Type)]++
		st.Events += out.Events
		if out.Status == "manual" {
			st.Manual++
		}
		how := string(out.How)
		if how == "" {
			how = "not_found"
			if out.Type != "none" {
				unknown[m.FromName]++
			}
		}
		st.Authors[how]++
	}
	stats, _ := json.Marshal(st)
	var errText *string
	if ierr != nil {
		s := ierr.Error()
		errText = &s
	}
	if _, err := pool.Exec(context.WithoutCancel(ctx), `UPDATE sync_runs SET finished_at = now(), ok = $2, stats = $3, error = $4 WHERE id = $1`,
		runID, ierr == nil, stats, errText); err != nil && ierr == nil {
		ierr = err
	}
	if ierr != nil {
		return ierr
	}
	log.Info("import-telegram: готово", "chat", exp.Name, "messages", st.Messages, "duplicates", st.Duplicates,
		"events", st.Events, "manual_queue", st.Manual, "types", st.Types, "authors", st.Authors, "took", time.Since(t0).Round(time.Second).String())
	if len(unknown) > 0 {
		names := make([]string, 0, len(unknown))
		for n := range unknown {
			names = append(names, n)
		}
		sort.Slice(names, func(a, b int) bool { return unknown[names[a]] > unknown[names[b]] })
		fmt.Println("\nАвтор не найден (имя аккаунта → сообщений про посещаемость). Добавьте их в seed/tg_aliases.csv:")
		for _, n := range names {
			fmt.Printf("  %-30s %d\n", n, unknown[n])
		}
	}
	return nil
}
