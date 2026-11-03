package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"sync"
	"time"

	"github.com/aa228058/tgqf/backend/internal/accounts"
	"github.com/aa228058/tgqf/backend/internal/config"
	"github.com/aa228058/tgqf/backend/internal/storage"
	"github.com/aa228058/tgqf/backend/internal/telegrammtgo"
	"github.com/aa228058/tgqf/backend/internal/messages"
	"github.com/mtgo-labs/mtgo/telegram/types"
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

func scheduleReconnect(state *reconnectState, now time.Time) time.Duration {
	state.attempts++
	delay := reconnectDelay(state.attempts)
	state.nextTry = now.Add(delay)
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
	messageService := messages.New(store.DB)
	var accountTenants sync.Map
	manager := telegrammtgo.NewManager(telegrammtgo.RuntimeConfig{APIID:cfg.TelegramAPIID, APIHash:cfg.TelegramAPIHash, ConnectTimeout:45*time.Second})
	defer func(){ if err := manager.Close(); err != nil { logger.Error("close Telegram clients", "error", err) } }()
	manager.SetMessageHandler(func(accountID string, message *types.Message, ownUserID int64, replyToOwn bool) {
		if message == nil || message.Chat == nil { return }
		chatType := ""
		switch message.Chat.Type {
		case types.ChatTypePrivate, types.ChatTypeDirect:
			chatType = "private"
		case types.ChatTypeGroup, types.ChatTypeSupergroup, types.ChatTypeForum:
			chatType = "group"
		case types.ChatTypeBot, types.ChatTypeChannel:
			return
		default:
			return
		}
		tenantValue, ok := accountTenants.Load(accountID)
		if !ok { return }
		textBody := message.Text
		if textBody == "" { textBody = message.Caption }
		mediaType := ""
		if message.Media != nil { mediaType = string(message.Media.MediaType()) }
		photoID := ""
		if message.Chat.Photo != nil {
			photoID = message.Chat.Photo.BigPhotoUniqueID
			if photoID == "" { photoID = message.Chat.Photo.SmallPhotoUniqueID }
		}
		_, err := messageService.Ingest(ctx, tenantValue.(string), messages.Incoming{
			AccountID: accountID, ChatID: message.ChatID, MessageID: int64(message.ID),
			ChatType: chatType, ChatTitle: message.Chat.FullName(), ChatUsername: message.Chat.Username,
			ChatMemberCount: message.Chat.MembersCount, ChatVerified: message.Chat.IsVerified, ChatPhotoID: photoID, SenderID: message.FromID,
			ReplyToMessageID: int64(message.ReplyToID), ReplyToOwnMessage: replyToOwn,
			IsOutgoing: message.Out || message.Outgoing, Text: textBody, MediaType: mediaType, MessageDate: message.Date,
		})
		if err != nil { logger.Warn("store filtered Telegram message", "account_id", accountID, "message_id", message.ID, "error", err) }
	})

	lastSavedSessions := make(map[string]string)
	reconnects := make(map[string]reconnectState)
	statusSynced := make(map[string]bool)
	ticker := time.NewTicker(30*time.Second)
	defer ticker.Stop()
	historyTicker := time.NewTicker(5*time.Minute)
	defer historyTicker.Stop()
	logger.Info("Telegram account worker started")
	for {
		if err := reconcile(ctx, accountService, manager, logger, lastSavedSessions, reconnects, statusSynced, &accountTenants, time.Now()); err != nil { logger.Error("reconcile Telegram accounts", "error", err) }
		processReplyJobs(ctx, messageService, manager, logger)
		select {
		case <-ctx.Done(): return
		case <-ticker.C:
		case <-historyTicker.C: processRecentHistory(ctx, messageService, manager, &accountTenants, logger)
		}
	}
}

func reconcile(ctx context.Context, accountsService *accounts.Service, manager *telegrammtgo.Manager, logger *slog.Logger, lastSavedSessions map[string]string, reconnects map[string]reconnectState, statusSynced map[string]bool, accountTenants *sync.Map, now time.Time) error {
	accountsList, err := accountsService.LoadAuthorizedRuntimeAccounts(ctx)
	if err != nil { return err }
	allowed := make(map[string]accounts.RuntimeAccount, len(accountsList))
	for _, account := range accountsList { allowed[account.ID] = account; accountTenants.Store(account.ID, account.TenantID) }
	accountTenants.Range(func(key, value any) bool { if _, ok := allowed[key.(string)]; !ok { accountTenants.Delete(key) }; return true })

	for _, id := range manager.ManagedAccountIDs() {
		if _, ok := allowed[id]; !ok {
			if err := manager.Stop(id); err != nil { logger.Warn("stop inactive Telegram account", "account_id", id, "error", err) }
			delete(lastSavedSessions, id)
			delete(reconnects, id)
			delete(statusSynced, id)
			accountTenants.Delete(id)
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
				delay := scheduleReconnect(&state, now)
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

func processReplyJobs(ctx context.Context, service *messages.Service, manager *telegrammtgo.Manager, logger *slog.Logger) {
	jobs,err:=service.ClaimPendingReplies(ctx,20)
	if err!=nil { logger.Error("claim Telegram reply jobs","error",err);return }
	for _,job:=range jobs {
		if ctx.Err()!=nil{return}
		if !manager.IsConnected(job.AccountID) {
			if err:=service.ReleaseReply(ctx,job,30*time.Second);err!=nil { logger.Error("release reply for offline Telegram account","job_id",job.ID,"error",err) }
			continue
		}
		sendCtx,cancel:=context.WithTimeout(ctx,45*time.Second)
		telegramMessageID,sendErr:=manager.SendReply(sendCtx,job.AccountID,job.ChatID,job.ReplyToMessageID,job.Text)
		cancel()
		if sendErr!=nil {
			if err:=service.FailReply(ctx,job,sendErr);err!=nil { logger.Error("reschedule failed Telegram reply","job_id",job.ID,"error",err) }
			logger.Warn("Telegram reply failed","job_id",job.ID,"account_id",job.AccountID,"attempt",job.Attempts,"error",sendErr)
			continue
		}
		if err:=service.CompleteReply(ctx,job,telegramMessageID);err!=nil {
			logger.Error("persist sent Telegram reply","job_id",job.ID,"telegram_message_id",telegramMessageID,"error",err)
			continue
		}
		logger.Info("Telegram reply sent","job_id",job.ID,"account_id",job.AccountID,"telegram_message_id",telegramMessageID)
	}
}


// processRecentHistory repairs short disconnect gaps for chats already known to
// the collector. It never enumerates all dialogs or imports unrelated traffic.
func processRecentHistory(ctx context.Context, service *messages.Service, manager *telegrammtgo.Manager, tenants *sync.Map, logger *slog.Logger) {
	tenants.Range(func(key, value any) bool {
		accountID, tenantID := key.(string), value.(string)
		if !manager.IsConnected(accountID) { return true }
		chatIDs, err := service.ListKnownChatIDs(ctx, tenantID, accountID, 20)
		if err != nil { logger.Warn("load known chats for history backfill", "account_id", accountID, "error", err); return true }
		for _, chatID := range chatIDs {
			if ctx.Err() != nil { return false }
			fetchCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			history, fetchErr := manager.GetChatHistory(fetchCtx, accountID, chatID, 50)
			if fetchErr != nil { cancel(); logger.Debug("history backfill skipped", "account_id", accountID, "chat_id", chatID, "error", fetchErr); continue }
			// Resolve all reply targets for this chat in one Telegram request
			// instead of issuing one RPC for every historical reply.
			replyIDs := make([]int32, 0)
			for _, message := range history {
				if message != nil && message.Chat != nil && message.ReplyToID > 0 {
					switch message.Chat.Type {
					case types.ChatTypeGroup, types.ChatTypeSupergroup, types.ChatTypeForum:
						replyIDs = append(replyIDs, message.ReplyToID)
					}
				}
			}
			replyTargets := manager.IsRepliesToOwnMessages(fetchCtx, accountID, chatID, replyIDs)
			for _, message := range history {
				if message == nil || message.Chat == nil || message.ID <= 0 { continue }
				chatType := ""
				switch message.Chat.Type {
				case types.ChatTypePrivate, types.ChatTypeDirect: chatType = "private"
				case types.ChatTypeGroup, types.ChatTypeSupergroup, types.ChatTypeForum: chatType = "group"
				default: continue
				}
				replyToOwn := chatType == "group" && replyTargets[message.ReplyToID]
				body := message.Text
				if body == "" { body = message.Caption }
				mediaType := ""
				if message.Media != nil { mediaType = string(message.Media.MediaType()) }
				photoID := ""
				if message.Chat.Photo != nil {
					photoID = message.Chat.Photo.BigPhotoUniqueID
					if photoID == "" { photoID = message.Chat.Photo.SmallPhotoUniqueID }
				}
				_, ingestErr := service.Ingest(fetchCtx, tenantID, messages.Incoming{
					AccountID: accountID, ChatID: chatID, MessageID: int64(message.ID),
					ChatType: chatType, ChatTitle: message.Chat.FullName(), ChatUsername: message.Chat.Username,
					ChatMemberCount: message.Chat.MembersCount, ChatVerified: message.Chat.IsVerified, ChatPhotoID: photoID,
					SenderID: message.FromID, ReplyToMessageID: int64(message.ReplyToID), ReplyToOwnMessage: replyToOwn,
					IsOutgoing: message.Out || message.Outgoing, Text: body, MediaType: mediaType, MessageDate: message.Date,
				})
				if ingestErr != nil { logger.Warn("store backfilled Telegram message", "account_id", accountID, "message_id", message.ID, "error", ingestErr) }
			}
			cancel()
		}
		return true
	})
}
