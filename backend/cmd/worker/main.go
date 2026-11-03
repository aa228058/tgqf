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

type reconnectState struct {
	attempts int
	nextTry time.Time
}

func reconnectDelay(attempt int) time.Duration {
	if attempt < 1 { attempt = 1 }
	delay := 30 * time.Second
	for i := 1; i < attempt && delay < 15*time.Minute; i++ { delay *= 2 }
	if delay > 15*time.Minute { return 15*time.Minute }
	return delay
}

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

	lastSavedSessions := make(map[string]string)
	reconnects := make(map[string]reconnectState)
	statusSynced := make(map[string]bool)
	ticker := time.NewTicker(30*time.Second)
	defer ticker.Stop()
	logger.Info("Telegram account worker started")
	for {
		if err := reconcile(ctx, accountService, manager, logger, lastSavedSessions, reconnects, statusSynced, time.Now()); err != nil { logger.Error("reconcile Telegram accounts", "error", err) }
		select { case <-ctx.Done(): return; case <-ticker.C: }
	}
}

func reconcile(ctx context.Context, accountsService *accounts.Service, manager *telegrammtgo.Manager, logger *slog.Logger, lastSavedSessions map[string]string, reconnects map[string]reconnectState, statusSynced map[string]bool, now time.Time) error {
	accountsList, err := accountsService.LoadAuthorizedRuntimeAccounts(ctx)
	if err != nil { return err }
	allowed := make(map[string]accounts.RuntimeAccount, len(accountsList))
	for _, account := range accountsList { allowed[account.ID] = account }

	for _, id := range manager.ManagedAccountIDs() {
		if _, ok := allowed[id]; !ok {
			if err := manager.Stop(id); err != nil { logger.Warn("stop inactive Telegram account", "account_id", id, "error", err) }
			delete(lastSavedSessions, id)
			delete(reconnects, id)
			delete(statusSynced, id)
		}
	}
	for id, account := range allowed {
		if manager.IsConnected(id) {
			delete(reconnects, id)
		} else {
			state := reconnects[id]
			if now.Before(state.nextTry) { continue }
			_ = manager.Stop(id)
			if err := manager.Start(id, account.Session, account.ProxyJSON); err != nil {
				state.attempts++
				delay := reconnectDelay(state.attempts)
				state.nextTry = now.Add(delay)
				reconnects[id] = state
				statusSynced[id] = false
				logger.Warn("Telegram account connection failed; retry scheduled", "account_id", id, "attempt", state.attempts, "retry_in", delay.String(), "error", err)
				if statusErr := accountsService.SetRuntimeStatus(ctx, account.TenantID, id, "error", err.Error()); statusErr != nil {
					logger.Error("update Telegram account error status", "account_id", id, "error", statusErr)
				}
				continue
			}
			delete(reconnects, id)
			lastSavedSessions[id] = account.Session
			logger.Info("Telegram account connected", "account_id", id)
		}

		// Retry a failed DB status update on later reconciliation ticks even
		// when the Telegram connection itself remains healthy.
		if !statusSynced[id] {
			if err := accountsService.SetRuntimeStatus(ctx, account.TenantID, id, "authorized", ""); err != nil {
				logger.Error("update Telegram account connected status", "account_id", id, "error", err)
			} else {
				statusSynced[id] = true
			}
		}

		// MTProto may refresh authorization/session data while the account is
		// connected. Export it from the live client and persist only changes.
		session, exportErr := manager.ExportSession(id)
		if exportErr != nil {
			logger.Warn("export Telegram session", "account_id", id, "error", exportErr)
			continue
		}
		if session == "" || session == lastSavedSessions[id] { continue }
		if err := accountsService.SaveRuntimeSession(ctx, account.TenantID, id, session); err != nil {
			logger.Error("persist refreshed Telegram session", "account_id", id, "error", err)
			continue
		}
		lastSavedSessions[id] = session
		logger.Info("persisted refreshed Telegram session", "account_id", id)
	}
	return nil
}
