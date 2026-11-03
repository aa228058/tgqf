package config

import (
	"fmt"
	"os"
	"strings"
)

// Config contains process-level settings. Secrets are read only from the environment.
type Config struct {
	HTTPAddr string
	AppEnv   string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr: strings.TrimSpace(os.Getenv("HTTP_ADDR")),
		AppEnv:   strings.TrimSpace(os.Getenv("APP_ENV")),
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if cfg.AppEnv == "" {
		cfg.AppEnv = "development"
	}
	if !strings.HasPrefix(cfg.HTTPAddr, ":") && !strings.Contains(cfg.HTTPAddr, ":") {
		return Config{}, fmt.Errorf("invalid HTTP_ADDR %q: expected host:port or :port", cfg.HTTPAddr)
	}
	return cfg, nil
}
