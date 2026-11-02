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
	// JIRA_FULL_SYNC_AT, по умолчанию 08:30 (Asia/Almaty) — раз в сутки полная: проекты, тимлиды, справочник сотрудников
	JiraFullSyncAt string
	// SYNC_HOURS, по умолчанию 08:30-20:00 (Asia/Almaty) — рабочее время: вне его worker не обновляет Jira и Bitrix
	// (бот Telegram принимает сообщения всегда). Пусто — без ограничения.
	SyncFrom, SyncTo int // минуты от полуночи; SyncFrom == SyncTo — круглосуточно

	BitrixVacationURL string // BITRIX_VACATION_URL, по умолчанию https://bitrix.avh.kz/api/v1/vacation_api.php
	BitrixAPIKey      string // BITRIX_API_KEY — секрет, идёт в query, в логах маскируется
	// BITRIX_SYNC_EVERY, по умолчанию 30m
	BitrixSyncEvery time.Duration

	TgBotToken string // TG_BOT_TOKEN — секрет, не логировать
	TgChatID   int64  // TG_CHAT_ID — группа «Adata Group» (отрицательное число)
	// TG_SATURDAY_AT, по умолчанию 20:30 (Алматы) — в пятницу в это время бот пишет в группу, кто отрабатывает в субботу; off — не писать
	TgSaturdayAt string

	// SECRETS_KEY — ключ шифрования секретов пользователей (токены личных Telegram-ботов руководителей), 16+ символов.
	// Не задан — уведомления только в колокольчике сервиса.
	SecretsKey string
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
		SecretsKey:        strings.TrimSpace(os.Getenv("SECRETS_KEY")),
		TgSaturdayAt:      getenv("TG_SATURDAY_AT", "20:30"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL не задан")
	}
	var err error
	if c.JiraSyncEvery, err = duration("JIRA_SYNC_EVERY", "30m"); err != nil {
		return c, err
	}
	c.JiraFullSyncAt = getenv("JIRA_FULL_SYNC_AT", "08:30")
	if _, err := time.Parse("15:04", c.JiraFullSyncAt); err != nil {
		return c, fmt.Errorf("JIRA_FULL_SYNC_AT: ожидается время ЧЧ:ММ, например 08:30")
	}
	if c.TgSaturdayAt != "off" {
		if _, err := time.Parse("15:04", c.TgSaturdayAt); err != nil {
			return c, fmt.Errorf("TG_SATURDAY_AT: ожидается время ЧЧ:ММ, например 20:30, или off")
		}
	}
	if c.SyncFrom, c.SyncTo, err = hoursRange(os.Getenv("SYNC_HOURS"), "08:30-20:00"); err != nil {
		return c, err
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

// InSyncHours — можно ли обновлять отчёты в момент t (по Алматы).
func (c Config) InSyncHours(t time.Time) bool {
	if c.SyncFrom == c.SyncTo {
		return true
	}
	loc, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		loc = time.FixedZone("Asia/Almaty", 5*3600)
	}
	t = t.In(loc)
	m := t.Hour()*60 + t.Minute()
	if c.SyncFrom < c.SyncTo {
		return m >= c.SyncFrom && m < c.SyncTo
	}
	return m >= c.SyncFrom || m < c.SyncTo // окно через полночь
}

// SyncHours — окно рабочего времени для логов, например «08:30-20:00».
func (c Config) SyncHours() string {
	if c.SyncFrom == c.SyncTo {
		return "круглосуточно"
	}
	return fmt.Sprintf("%02d:%02d-%02d:%02d", c.SyncFrom/60, c.SyncFrom%60, c.SyncTo/60, c.SyncTo%60)
}

// hoursRange разбирает «ЧЧ:ММ-ЧЧ:ММ»; "-" или "off" — круглосуточно.
func hoursRange(v, def string) (int, int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		v = def
	}
	if v == "-" || strings.EqualFold(v, "off") {
		return 0, 0, nil
	}
	a, b, ok := strings.Cut(v, "-")
	ta, errA := time.Parse("15:04", strings.TrimSpace(a))
	tb, errB := time.Parse("15:04", strings.TrimSpace(b))
	if !ok || errA != nil || errB != nil {
		return 0, 0, fmt.Errorf("SYNC_HOURS: ожидается «ЧЧ:ММ-ЧЧ:ММ», например %s, или off", def)
	}
	return ta.Hour()*60 + ta.Minute(), tb.Hour()*60 + tb.Minute(), nil
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
