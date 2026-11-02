// Command reports — единый бинарь сервиса: serve | worker | migrate | seed | sync | bitrix | users | import-telegram | healthcheck.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

const usage = `использование: reports <команда> [флаги]

команды:
  serve        HTTP API на HTTP_ADDR (по умолчанию :8080)
  worker       фоновые задачи по расписанию
  migrate      применить миграции БД (goose up)
  seed --dir   загрузить сотрудников, алиасы Telegram и праздники из CSV
  sync jira    синхронизировать задачи открытых спринтов из Jira (--full)
  sync bitrix  синхронизировать отпуска из API отпусков Bitrix
  bitrix probe --id <табельный ID>  сырой ответ API отпусков (ключ замаскирован)
  users add --login --name --role  создать пользователя (первого admin; остальных — в «Настройках»)
  import-telegram --file <result.json | папка HTML>  история группы из экспорта Telegram Desktop
  tg saturday [--send]  кто отрабатывает в субботу: без --send — показать текст, с --send — отправить в группу
  healthcheck  проверить /api/v1/healthz локального serve (для HEALTHCHECK)
`

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "serve":
		err = runServe(ctx, log)
	case "worker":
		err = runWorker(ctx, log)
	case "migrate":
		err = runMigrate(ctx, log)
	case "seed":
		err = runSeed(ctx, log, args)
	case "sync":
		err = runSync(ctx, log, args)
	case "bitrix":
		err = runBitrix(ctx, args)
	case "users":
		err = runUsers(ctx, log, args)
	case "import-telegram":
		err = runImportTelegram(ctx, log, args)
	case "tg":
		err = runTg(ctx, log, args)
	case "healthcheck":
		err = runHealthcheck(ctx)
	case "-h", "--help", "help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "неизвестная команда %q\n\n%s", cmd, usage)
		os.Exit(2)
	}
	if err != nil {
		log.Error(cmd+" завершился с ошибкой", "err", err)
		os.Exit(1)
	}
}
