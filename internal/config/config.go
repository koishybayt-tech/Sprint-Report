// Package config читает настройки из переменных окружения.
package config

import (
	"errors"
	"os"
)

type Config struct {
	DatabaseURL string // DATABASE_URL, обязателен
	HTTPAddr    string // HTTP_ADDR, по умолчанию :8080
}

func Load() (Config, error) {
	c := Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		HTTPAddr:    getenv("HTTP_ADDR", ":8080"),
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL не задан")
	}
	return c, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
