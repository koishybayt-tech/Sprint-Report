// Package config читает настройки из переменных окружения.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL string // DATABASE_URL, обязателен
	HTTPAddr    string // HTTP_ADDR, по умолчанию :8080
	JiraBaseURL string // JIRA_BASE_URL, по умолчанию https://jira.adata.kz
	JiraPAT     string // JIRA_PAT — секрет, не логировать
	// JIRA_SYNC_EVERY, по умолчанию 30m — инкрементальная синхронизация (только изменённые задачи)
	JiraSyncEvery time.Duration
	// JIRA_FULL_SYNC_AT, по умолчанию 03:00 (Asia/Almaty) — раз в сутки полная: проекты, тимлиды, справочник сотрудников
	JiraFullSyncAt string

	BitrixVacationURL string // BITRIX_VACATION_URL, по умолчанию https://bitrix.avh.kz/api/v1/vacation_api.php
	BitrixAPIKey      string // BITRIX_API_KEY — секрет, идёт в query, в логах маскируется
	// BITRIX_SYNC_EVERY, по умолчанию 30m
	BitrixSyncEvery time.Duration

	TgBotToken string // TG_BOT_TOKEN — секрет, не логировать
	TgChatID   int64  // TG_CHAT_ID — группа «Adata Group» (отрицательное число)
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		HTTPAddr:          getenv("HTTP_ADDR", ":8080"),
		JiraBaseURL:       strings.TrimRight(getenv("JIRA_BASE_URL", "https://jira.adata.kz"), "/"),
		JiraPAT:           strings.TrimSpace(os.Getenv("JIRA_PAT")),
		BitrixVacationURL: getenv("BITRIX_VACATION_URL", "https://bitrix.avh.kz/api/v1/vacation_api.php"),
		BitrixAPIKey:      strings.TrimSpace(os.Getenv("BITRIX_API_KEY")),
		TgBotToken:        strings.TrimSpace(os.Getenv("TG_BOT_TOKEN")),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL не задан")
	}
	var err error
	if c.JiraSyncEvery, err = duration("JIRA_SYNC_EVERY", "30m"); err != nil {
		return c, err
	}
	c.JiraFullSyncAt = getenv("JIRA_FULL_SYNC_AT", "03:00")
	if _, err := time.Parse("15:04", c.JiraFullSyncAt); err != nil {
		return c, fmt.Errorf("JIRA_FULL_SYNC_AT: ожидается время ЧЧ:ММ, например 03:00")
	}
	if c.BitrixSyncEvery, err = duration("BITRIX_SYNC_EVERY", "30m"); err != nil {
		return c, err
	}
	if v := strings.TrimSpace(os.Getenv("TG_CHAT_ID")); v != "" {
		if c.TgChatID, err = strconv.ParseInt(v, 10, 64); err != nil {
			return c, fmt.Errorf("TG_CHAT_ID: ожидается число, например -459179610")
		}
	}
	return c, nil
}

func duration(key, def string) (time.Duration, error) {
	d, err := time.ParseDuration(getenv(key, def))
	if err != nil || d < time.Minute {
		return 0, fmt.Errorf("%s: ожидается длительность не меньше 1m, например %s", key, def)
	}
	return d, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
