package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/adata/reports/internal/api"
	"github.com/adata/reports/internal/bitrix"
	"github.com/adata/reports/internal/config"
	"github.com/adata/reports/internal/db"
	"github.com/adata/reports/internal/jira"
	"github.com/adata/reports/internal/tgbot"
)

func runServe(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewRouter(pool, log, cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("serve: слушаю", "addr", cfg.HTTPAddr)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("serve: остановка")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// runWorker синхронизирует Jira (каждые JIRA_SYNC_EVERY, 30m: изменённые задачи; раз в сутки после JIRA_FULL_SYNC_AT — полная)
// историю задач года для карточки сотрудника (каждые JIRA_SYNC_EVERY, первая загрузка — полная),
// всё это — только в рабочее время SYNC_HOURS (08:30–20:00 по Алматы),
// и отпуска Bitrix (каждые BITRIX_SYNC_EVERY, 30m),
// оба раза — сразу при старте, и принимает сообщения группы ботом Telegram.
// Без JIRA_PAT / BITRIX_API_KEY / TG_BOT_TOKEN+TG_CHAT_ID соответствующая часть выключена.
func runWorker(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	client, err := jira.New(cfg.JiraBaseURL, cfg.JiraPAT)
	if err != nil {
		log.Warn("worker: синхронизация Jira выключена", "reason", err.Error())
	}
	bx, err := bitrix.New(cfg.BitrixVacationURL, cfg.BitrixAPIKey)
	if err != nil {
		log.Warn("worker: синхронизация Bitrix выключена", "reason", err.Error())
	}
	if cfg.TgBotToken != "" && cfg.TgChatID != 0 {
		if store, err := tgbot.NewStore(ctx, pool); err != nil {
			log.Error("worker: бот Telegram выключен — справочник авторов", "err", err)
		} else if bot, err := tgbot.NewBot(cfg.TgBotToken, cfg.TgChatID, store, log); err != nil {
			log.Warn("worker: бот Telegram выключен", "reason", err.Error())
		} else {
			go bot.Run(ctx)
		}
	} else {
		log.Warn("worker: бот Telegram выключен", "reason", "нет TG_BOT_TOKEN или TG_CHAT_ID")
	}
	log.Info("worker: запущен", "jira_sync_every", cfg.JiraSyncEvery.String(), "bitrix_sync_every", cfg.BitrixSyncEvery.String())
	// Вне рабочего времени (SYNC_HOURS) отчёты не обновляются; первый проход после начала дня догоняет всё,
	// в том числе суточную полную синхронизацию Jira. Смена режима пишется в лог один раз.
	offHours := false
	inHours := func() bool {
		in := cfg.InSyncHours(time.Now())
		if in == offHours {
			offHours = !in
			if offHours {
				log.Info("worker: нерабочее время — обновления Jira и Bitrix на паузе", "sync_hours", cfg.SyncHours())
			} else {
				log.Info("worker: рабочее время — обновления возобновлены")
			}
		}
		return in
	}
	runJira := func() {
		if client == nil || !inHours() {
			return
		}
		full, err := fullSyncDue(ctx, pool, cfg.JiraFullSyncAt)
		if err != nil {
			log.Error("worker: синхронизация Jira", "err", err)
			return
		}
		if err := syncJira(ctx, log, cfg, client, pool, full); err != nil && ctx.Err() == nil {
			log.Error("worker: синхронизация Jira", "err", err)
		}
	}
	runBitrix := func() {
		if bx == nil || !inHours() {
			return
		}
		if err := syncBitrix(ctx, log, bx, client, pool); err != nil && ctx.Err() == nil {
			log.Error("worker: синхронизация Bitrix", "err", err)
		}
	}
	runJira()
	runBitrix()
	// Личные Telegram-боты руководителей: привязка по /start <код> и кнопка «Игнорировать».
	go notifier(cfg, pool, log).RunBots(ctx, 10*time.Second)
	// История задач за год (карточка сотрудника) — отдельно: первая загрузка идёт ~20 минут и не должна держать остальное.
	if client != nil {
		go func() {
			t := time.NewTicker(cfg.JiraSyncEvery)
			defer t.Stop()
			for {
				if cfg.InSyncHours(time.Now()) {
					if err := syncJiraHistory(ctx, log, client, pool, false); err != nil && ctx.Err() == nil {
						log.Error("worker: история Jira", "err", err)
					}
				}
				select {
				case <-ctx.Done():
					return
				case <-t.C:
				}
			}
		}()
	}

	heartbeat := time.NewTicker(time.Minute)
	defer heartbeat.Stop()
	jiraTick := time.NewTicker(cfg.JiraSyncEvery)
	defer jiraTick.Stop()
	bitrixTick := time.NewTicker(cfg.BitrixSyncEvery)
	defer bitrixTick.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Info("worker: остановка")
			return nil
		case <-heartbeat.C:
			log.Info("worker: жив")
		case <-jiraTick.C:
			runJira()
		case <-bitrixTick.C:
			runBitrix()
		}
	}
}

func runMigrate(ctx context.Context, log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}
	log.Info("migrate: готово")
	return nil
}

// runHealthcheck дёргает healthz локального serve; используется в HEALTHCHECK (в distroless нет curl).
func runHealthcheck(ctx context.Context) error {
	port := "8080"
	if addr := os.Getenv("HTTP_ADDR"); len(addr) > 1 && addr[0] == ':' {
		port = addr[1:]
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:"+port+"/api/v1/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: HTTP %d", resp.StatusCode)
	}
	return nil
}
