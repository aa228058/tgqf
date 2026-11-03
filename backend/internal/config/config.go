package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	HTTPAddr    string
	AppEnv      string
	DatabaseURL string
	RedisURL    string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:    env("HTTP_ADDR", ":8080"),
		AppEnv:      env("APP_ENV", "development"),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		RedisURL:    strings.TrimSpace(os.Getenv("REDIS_URL")),
	}
	if _, _, found := strings.Cut(cfg.HTTPAddr, ":"); !found {
		return Config{}, fmt.Errorf("invalid HTTP_ADDR %q: expected host:port or :port", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL != "" {
		u, err := url.Parse(cfg.DatabaseURL)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
			return Config{}, fmt.Errorf("DATABASE_URL must be a valid postgres URL")
		}
	}
	if cfg.RedisURL != "" {
		u, err := url.Parse(cfg.RedisURL)
		if err != nil || (u.Scheme != "redis" && u.Scheme != "rediss") {
			return Config{}, fmt.Errorf("REDIS_URL must use redis:// or rediss://")
		}
	}
	if (cfg.DatabaseURL == "") != (cfg.RedisURL == "") {
		return Config{}, fmt.Errorf("DATABASE_URL and REDIS_URL must be configured together")
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
