package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/adata/reports/internal/auth"
	"github.com/adata/reports/internal/config"
	"github.com/adata/reports/internal/db"
)

// runUsers: reports users add --login <логин> --name <ФИО> --role admin [--employee <логин Jira>].
// Нужна для первого администратора; остальных удобнее заводить в UI («Настройки»).
// Пароль берётся из REPORTS_NEW_PASSWORD, а если её нет — генерируется и печатается один раз.
func runUsers(ctx context.Context, log *slog.Logger, args []string) error {
	if len(args) == 0 || args[0] != "add" {
		return errors.New("использование: reports users add --login <логин> --name <ФИО> --role admin|lead|hr|employee [--employee <логин Jira>]")
	}
	fs := flag.NewFlagSet("users add", flag.ContinueOnError)
	login := fs.String("login", "", "логин для входа")
	name := fs.String("name", "", "ФИО")
	role := fs.String("role", auth.RoleEmployee, "роль: admin | lead | hr | employee")
	emp := fs.String("employee", "", "логин сотрудника в Jira для связи с карточкой (необязательно)")
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

	password, generated := os.Getenv("REPORTS_NEW_PASSWORD"), false
	if password == "" {
		password, generated = auth.GeneratePassword(), true
	}
	u, err := auth.NewStore(pool).Create(ctx, *login, *name, *role, password, *emp)
	if err != nil {
		return err
	}
	log.Info("users: создан", "id", u.ID, "login", u.Login, "role", u.Role, "employee_id", u.EmployeeID)
	if generated {
		fmt.Printf("Пароль для %s (показывается один раз): %s\n", u.Login, password)
	}
	return nil
}
