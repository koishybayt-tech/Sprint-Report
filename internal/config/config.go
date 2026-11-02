// Package config читает настройки из переменных окружения.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL string // DATABASE_URL, обязателен
	HTTPAddr    string // HTTP_ADDR, по умолчанию :8080
	JiraBaseURL string // JIRA_BASE_URL, по умолчанию https://jira.adata.kz
	JiraPAT     string // JIRA_PAT — секрет, не логировать
	// JIRA_SYNC_EVERY, по умолчанию 15m
	JiraSyncEvery time.Duration
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
		JiraBaseURL: strings.TrimRight(getenv("JIRA_BASE_URL", "https://jira.adata.kz"), "/"),
		JiraPAT:     strings.TrimSpace(os.Getenv("JIRA_PAT")),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL не задан")
	}
	every, err := time.ParseDuration(getenv("JIRA_SYNC_EVERY", "15m"))
	if err != nil || every < time.Minute {
		return c, fmt.Errorf("JIRA_SYNC_EVERY: ожидается длительность не меньше 1m, например 15m")
	}
	c.JiraSyncEvery = every
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
