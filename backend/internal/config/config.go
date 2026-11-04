package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr           string
	AppEnv             string
	DatabaseURL        string
	RedisURL           string
	SuperAdminUsername string
	SuperAdminPassword string
	TelegramAPIID      int32
	TelegramAPIHash    string
}

func Load() (Config, error) {
	cfg := Config{HTTPAddr: env("HTTP_ADDR", ":8080"), AppEnv: env("APP_ENV", "development"), DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")), RedisURL: strings.TrimSpace(os.Getenv("REDIS_URL")), SuperAdminUsername: strings.TrimSpace(os.Getenv("SUPER_ADMIN_USERNAME")), SuperAdminPassword: os.Getenv("SUPER_ADMIN_PASSWORD"), TelegramAPIHash: strings.TrimSpace(os.Getenv("TELEGRAM_API_HASH"))}
	if _, _, ok := strings.Cut(cfg.HTTPAddr, ":"); !ok {
		return Config{}, fmt.Errorf("invalid HTTP_ADDR %q: expected host:port or :port", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL != "" {
		u, e := url.Parse(cfg.DatabaseURL)
		if e != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
			return Config{}, fmt.Errorf("DATABASE_URL must be a valid postgres URL")
		}
	}
	if cfg.RedisURL != "" {
		u, e := url.Parse(cfg.RedisURL)
		if e != nil || (u.Scheme != "redis" && u.Scheme != "rediss") {
			return Config{}, fmt.Errorf("REDIS_URL must use redis:// or rediss://")
		}
	}
	if (cfg.DatabaseURL == "") != (cfg.RedisURL == "") {
		return Config{}, fmt.Errorf("DATABASE_URL and REDIS_URL must be configured together")
	}
	if (cfg.SuperAdminUsername == "") != (cfg.SuperAdminPassword == "") {
		return Config{}, fmt.Errorf("SUPER_ADMIN_USERNAME and SUPER_ADMIN_PASSWORD must be configured together")
	}
	apiIDRaw := strings.TrimSpace(os.Getenv("TELEGRAM_API_ID"))
	if (apiIDRaw == "") != (cfg.TelegramAPIHash == "") {
		return Config{}, fmt.Errorf("TELEGRAM_API_ID and TELEGRAM_API_HASH must be configured together")
	}
	if apiIDRaw != "" {
		n, e := strconv.ParseInt(apiIDRaw, 10, 32)
		if e != nil || n <= 0 {
			return Config{}, fmt.Errorf("TELEGRAM_API_ID must be a positive 32-bit integer")
		}
		cfg.TelegramAPIID = int32(n)
	}
	return cfg, nil
}
func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
