package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aa228058/tgqf/backend/internal/accounts"
	"github.com/aa228058/tgqf/backend/internal/config"
	"github.com/aa228058/tgqf/backend/internal/storage"
	"github.com/aa228058/tgqf/backend/internal/telegrammtgo"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	cfg, err := config.Load()
	if err != nil { logger.Error("load configuration", "error", err); os.Exit(1) }
	if cfg.DatabaseURL == "" || cfg.RedisURL == "" { logger.Error("worker requires DATABASE_URL and REDIS_URL"); os.Exit(1) }
	if cfg.TelegramAPIID <= 0 || cfg.TelegramAPIHash == "" { logger.Error("worker requires TELEGRAM_API_ID and TELEGRAM_API_HASH"); os.Exit(1) }

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	store, err := storage.Open(ctx, cfg.DatabaseURL, cfg.RedisURL)
	if err != nil { logger.Error("initialize worker storage", "error", err); os.Exit(1) }
	defer store.Close()

	accountService := accounts.New(store.DB)
	manager := telegrammtgo.NewManager(telegrammtgo.RuntimeConfig{APIID:cfg.TelegramAPIID, APIHash:cfg.TelegramAPIHash, ConnectTimeout:45*time.Second})
	defer func(){ if err := manager.Close(); err != nil { logger.Error("close Telegram clients", "error", err) } }()

	ticker := time.NewTicker(30*time.Second)
	defer ticker.Stop()
	logger.Info("Telegram account worker started")
	for {
		if err := reconcile(ctx, accountService, manager, logger); err != nil { logger.Error("reconcile Telegram accounts", "error", err) }
		select { case <-ctx.Done(): return; case <-ticker.C: }
	}
}

func reconcile(ctx context.Context, accountsService *accounts.Service, manager *telegrammtgo.Manager, logger *slog.Logger) error {
	accountsList, err := accountsService.LoadAuthorizedRuntimeAccounts(ctx)
	if err != nil { return err }
	allowed := make(map[string]accounts.RuntimeAccount, len(accountsList))
	for _, account := range accountsList { allowed[account.ID] = account }

	for _, id := range manager.ManagedAccountIDs() {
		if _, ok := allowed[id]; !ok {
			if err := manager.Stop(id); err != nil { logger.Warn("stop inactive Telegram account", "account_id", id, "error", err) }
		}
	}
	for id, account := range allowed {
		if manager.IsConnected(id) { continue }
		if err := manager.Start(id, account.Session, account.ProxyJSON); err != nil {
			logger.Warn("Telegram account connection failed", "account_id", id, "error", err)
			_ = accountsService.SetRuntimeStatus(ctx, account.TenantID, id, "error", err.Error())
			continue
		}
		_ = accountsService.SetRuntimeStatus(ctx, account.TenantID, id, "authorized", "")
	}
	return nil
}

