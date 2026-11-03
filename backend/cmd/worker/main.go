package main

import (
	cryptorand "crypto/rand"
	"errors"
	"context"
	"encoding/hex"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"sync"
	"time"

	"github.com/aa228058/tgqf/backend/internal/accounts"
	"github.com/aa228058/tgqf/backend/internal/config"
	"github.com/aa228058/tgqf/backend/internal/storage"
	"github.com/aa228058/tgqf/backend/internal/telegrammtgo"
	"github.com/aa228058/tgqf/backend/internal/messages"
	"github.com/mtgo-labs/mtgo/telegram/types"
	"github.com/mtgo-labs/mtgo/tgerr"
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
	messageService := messages.New(store.DB, store.Redis)
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
		case types.ChatTypeChannel:
			chatType = "channel"
		case types.ChatTypeBot:
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
		ingestCtx, cancelIngest := context.WithTimeout(ctx, 5*time.Second)
		defer cancelIngest()
		_, err := messageService.Ingest(ingestCtx, tenantValue.(string), messages.Incoming{
			AccountID: accountID, ChatID: message.ChatID, MessageID: int64(message.ID),
			ChatType: chatType, ChatTitle: message.Chat.FullName(), ChatUsername: message.Chat.Username,
			ChatMemberCount: message.Chat.MembersCount, ChatVerified: message.Chat.IsVerified, SenderID: message.FromID, SenderUsername: func() string { if message.Sender != nil { return message.Sender.Username }; return "" }(), SenderName: func() string { if message.Sender != nil { return message.Sender.FullName() }; return "" }(),
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
	workerID := newWorkerInstanceID()
	hostname, _ := os.Hostname()
	if err := recordWorkerHeartbeat(ctx, store.DB, workerID, hostname, os.Getpid(), true); err != nil {
		logger.Error("register worker heartbeat", "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := recordWorkerHeartbeat(shutdownCtx, store.DB, workerID, hostname, os.Getpid(), false); err != nil {
			logger.Warn("mark worker stopped", "error", err)
		}
	}()
	logger.Info("Telegram account worker started", "worker_id", workerID, "hostname", hostname)
	// Heartbeats run independently from queue processing. Telegram RPCs can
	// legitimately take longer than one heartbeat interval; they must not make
	// a live worker look stale merely because the main loop is busy.
	go func() {
		heartbeatTicker := time.NewTicker(10 * time.Second)
		defer heartbeatTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeatTicker.C:
				heartbeatCtx, cancelHeartbeat := context.WithTimeout(ctx, 3*time.Second)
				if err := recordWorkerHeartbeat(heartbeatCtx, store.DB, workerID, hostname, os.Getpid(), true); err != nil {
					logger.Error("persist worker heartbeat", "worker_id", workerID, "error", err)
				}
				cancelHeartbeat()
			}
		}
	}()
	for {
		if err := reconcile(ctx, accountService, manager, logger, lastSavedSessions, reconnects, statusSynced, &accountTenants, time.Now()); err != nil { logger.Error("reconcile Telegram accounts", "error", err) }
		processReplyJobs(ctx, messageService, manager, logger)
		processKeywordActionJobs(ctx, messageService, manager, logger)
		processJoinTasks(ctx, messageService, manager, logger, &accountTenants)
		processJoinApprovalSync(ctx, messageService, manager, logger, &accountTenants)
		processProfileUpdateJobs(ctx, accountService, messageService, manager, logger, &accountTenants)
		select {
		case <-ctx.Done(): return
		case <-ticker.C:
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
			if account.ReconnectAttempts > 0 {
				if err := accountsService.ClearRuntimeReconnectState(ctx, account.TenantID, id); err != nil {
					logger.Warn("clear Telegram reconnect state", "account_id", id, "error", err)
				}
			}
		} else {
			state, exists := reconnects[id]
			if !exists && account.ReconnectAttempts > 0 {
				state = reconnectState{attempts: account.ReconnectAttempts, nextTry: account.ReconnectAt}
				reconnects[id] = state
			}
			if now.Before(state.nextTry) { continue }
			_ = manager.Stop(id)
			if err := manager.Start(id, account.Session, account.ProxyJSON, account.DeviceProfile); err != nil {
				delay := scheduleReconnect(&state, now)
				reconnects[id] = state
				if persistErr := accountsService.SaveRuntimeReconnectState(ctx, account.TenantID, id, state.attempts, state.nextTry); persistErr != nil {
					logger.Error("persist Telegram reconnect state", "account_id", id, "error", persistErr)
				}
				statusSynced[id] = false
				logger.Warn("Telegram account connection failed; retry scheduled", "account_id", id, "attempt", state.attempts, "retry_in", delay.String(), "error", err)
				if statusErr := accountsService.SetRuntimeStatus(ctx, account.TenantID, id, "error", err.Error()); statusErr != nil {
					logger.Error("update Telegram account error status", "account_id", id, "error", statusErr)
				}
				continue
			}
			delete(reconnects, id)
			if clearErr := accountsService.ClearRuntimeReconnectState(ctx, account.TenantID, id); clearErr != nil {
				logger.Warn("clear Telegram reconnect state", "account_id", id, "error", clearErr)
			}
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
		// A tenant may expire or be disabled after a job was claimed. Recheck
		// immediately before any Telegram side effect and freeze the job.
		if !service.TenantCanOperate(ctx,job.TenantID) {
			if err:=service.PauseReply(ctx,job,"tenant inactive; manual review required");err!=nil { logger.Error("pause reply for inactive tenant","job_id",job.ID,"error",err) }
			continue
		}
		if !manager.IsConnected(job.AccountID) {
			if err:=service.ReleaseReply(ctx,job,30*time.Second);err!=nil { logger.Error("release reply for offline Telegram account","job_id",job.ID,"error",err) }
			continue
		}
		sendCtx,cancel:=context.WithTimeout(ctx,45*time.Second)
		var telegramMessageID int64
		var sendErr error
		if job.MessageType=="contact" {
			telegramMessageID,sendErr=manager.SendContactReply(sendCtx,job.AccountID,job.ChatID,job.ReplyToMessageID,job.ContactPhone,job.ContactFirstName,job.ContactLastName)
		} else {
			telegramMessageID,sendErr=manager.SendReply(sendCtx,job.AccountID,job.ChatID,job.ReplyToMessageID,job.Text)
		}
		cancel()
		if sendErr!=nil {
			floodWait,_:=tgerr.AsFloodWait(sendErr)
			if floodWait>0 {
				logger.Warn("Telegram imposed FloodWait; preserving server cooldown","job_id",job.ID,"account_id",job.AccountID,"wait",floodWait.String())
			}
			if err:=service.FailReplyWithFloodWait(ctx,job,sendErr,floodWait);err!=nil { logger.Error("reschedule failed Telegram reply","job_id",job.ID,"error",err) }
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


func processKeywordActionJobs(ctx context.Context, service *messages.Service, manager *telegrammtgo.Manager, logger *slog.Logger) {
    jobs,err:=service.ClaimKeywordActionJobs(ctx,20)
    if err!=nil {logger.Error("claim keyword action jobs","error",err);return}
    for _,job:=range jobs {
        if ctx.Err()!=nil{return}
        if !service.TenantCanOperate(ctx,job.TenantID) {
            if err:=service.PauseKeywordAction(ctx,job,"tenant inactive; manual review required");err!=nil{logger.Error("pause keyword action for inactive tenant","job_id",job.ID,"error",err)}
            continue
        }
        if !manager.IsConnected(job.AccountID) {
            _=service.FinishKeywordAction(ctx,job,0,errors.New("Telegram account offline"),0)
            continue
        }
        sendCtx,cancel:=context.WithTimeout(ctx,45*time.Second)
        telegramMessageID,sendErr:=manager.SendReply(sendCtx,job.AccountID,job.ChatID,job.ReplyToMessageID,job.Text)
        cancel()
        floodWait,_:=tgerr.AsFloodWait(sendErr)
        if err:=service.FinishKeywordAction(ctx,job,telegramMessageID,sendErr,floodWait);err!=nil {
            logger.Error("persist keyword action result","job_id",job.ID,"error",err)
            continue
        }
        if sendErr!=nil {logger.Warn("keyword action send failed","job_id",job.ID,"account_id",job.AccountID,"error",sendErr)} else {
            logger.Info("keyword action sent","job_id",job.ID,"account_id",job.AccountID,"telegram_message_id",telegramMessageID)
        }
    }
}

func processJoinTasks(ctx context.Context, service *messages.Service, manager *telegrammtgo.Manager, logger *slog.Logger, accountTenants *sync.Map) {
    tenantSet:=map[string]bool{}
    accountTenants.Range(func(_,value any)bool{if tenantID,ok:=value.(string);ok&&tenantID!=""{tenantSet[tenantID]=true};return true})
    for tenantID:=range tenantSet {
        jobs,err:=service.ClaimJoinTaskAccounts(ctx,tenantID,10)
        if err!=nil {logger.Error("claim Telegram join tasks","tenant_id",tenantID,"error",err);continue}
        blockedTasks:=map[string]bool{}
        blockedAccounts:=map[string]bool{}
        for _,job:=range jobs {
            if ctx.Err()!=nil{return}
            if blockedTasks[job.TaskID]||blockedAccounts[job.AccountID]{continue}
            if !service.TenantCanOperate(ctx,job.TenantID) {
                if err:=service.FailJoinTaskAccount(ctx,job,"tenant subscription inactive; task requires manual review");err!=nil{logger.Error("close join task for inactive tenant","task_id",job.TaskID,"error",err)}
                continue
            }
            if !manager.IsConnected(job.AccountID) {
                if err:=service.RescheduleJoinTaskAccount(ctx,job,30*time.Second,"Telegram account is temporarily disconnected");err!=nil{logger.Error("reschedule join task for offline account","task_id",job.TaskID,"error",err)}
                blockedTasks[job.TaskID]=true;blockedAccounts[job.AccountID]=true
                continue
            }
            joinCtx,cancel:=context.WithTimeout(ctx,60*time.Second)
            chatID,joinErr:=manager.JoinChat(joinCtx,job.AccountID,job.TargetURL)
            cancel()
            if joinErr!=nil {
                if wait,ok:=tgerr.AsFloodWait(joinErr);ok {
                    logger.Warn("Telegram join FloodWait; preserving server cooldown","task_id",job.TaskID,"account_id",job.AccountID,"wait",wait.String())
                    if err:=service.RescheduleJoinTaskAccount(ctx,job,wait,joinErr.Error());err!=nil{logger.Error("reschedule join task after FloodWait","task_id",job.TaskID,"error",err)}
                    blockedTasks[job.TaskID]=true;blockedAccounts[job.AccountID]=true
                    continue
                }
                upper:=strings.ToUpper(joinErr.Error())
                if tgerr.IsInviteRequestSent(joinErr) || strings.Contains(upper,"INVITE_REQUEST_SENT") || strings.Contains(upper,"INVITEREQUESTSENT") {
                    if err:=service.CompleteJoinTaskAccount(ctx,job,0,"request_pending");err!=nil{logger.Error("record pending Telegram join request","task_id",job.TaskID,"error",err)}
                    continue
                }
                if strings.Contains(upper,"USER_ALREADY_PARTICIPANT")||strings.Contains(upper,"ALREADY_PARTICIPANT") {
                    if err:=service.CompleteJoinTaskAccount(ctx,job,0,"already_member");err!=nil{logger.Error("record existing Telegram membership","task_id",job.TaskID,"error",err)}
                    continue
                }
                if err:=service.FailJoinTaskAccount(ctx,job,joinErr.Error());err!=nil{logger.Error("record failed Telegram join","task_id",job.TaskID,"error",err)}
                logger.Warn("Telegram group join failed","task_id",job.TaskID,"account_id",job.AccountID,"error",joinErr)
                continue
            }
            if err:=service.CompleteJoinTaskAccount(ctx,job,chatID,"not_required");err!=nil{logger.Error("persist Telegram group join","task_id",job.TaskID,"chat_id",chatID,"error",err);continue}
            logger.Info("Telegram group joined","task_id",job.TaskID,"account_id",job.AccountID,"chat_id",chatID)
        }
    }
}

func processJoinApprovalSync(ctx context.Context, service *messages.Service, manager *telegrammtgo.Manager, logger *slog.Logger, accountTenants *sync.Map) {
	tenantSet:=map[string]bool{}
	accountTenants.Range(func(_,value any)bool{if tenantID,ok:=value.(string);ok&&tenantID!=""{tenantSet[tenantID]=true};return true})
	for tenantID:=range tenantSet {
		jobs,err:=service.ListPendingJoinApprovals(ctx,tenantID,50)
		if err!=nil {logger.Error("list pending Telegram join approvals","tenant_id",tenantID,"error",err);continue}
		for _,job:=range jobs {
			if ctx.Err()!=nil{return}
			if !manager.IsConnected(job.AccountID) {continue}
			checkCtx,cancel:=context.WithTimeout(ctx,20*time.Second)
			var memberStatus string
			var checkErr error
			if job.InviteHash!="" {
				memberStatus,checkErr=manager.CheckPrivateInviteApproval(checkCtx,job.AccountID,job.InviteHash)
			} else if job.ChatID>0 {
				memberStatus,checkErr=manager.CheckOwnChatMembership(checkCtx,job.AccountID,job.ChatID)
			} else {
				cancel()
				continue
			}
			cancel()
			if checkErr!=nil {
				if wait,ok:=tgerr.AsFloodWait(checkErr);ok {logger.Warn("Telegram approval sync FloodWait; deferring checks","account_id",job.AccountID,"wait",wait.String());continue}
				logger.Warn("check Telegram join approval","task_id",job.TaskID,"account_id",job.AccountID,"error",checkErr)
				continue
			}
			if err:=service.SyncJoinApproval(ctx,job,strings.ToLower(memberStatus));err!=nil {logger.Error("persist Telegram join approval status","task_id",job.TaskID,"account_id",job.AccountID,"error",err)}
		}
	}
}

func processProfileUpdateJobs(ctx context.Context, accountService *accounts.Service, messageService *messages.Service, manager *telegrammtgo.Manager, logger *slog.Logger, accountTenants *sync.Map) {
	if err:=accountService.PauseInactiveProfileUpdateJobs(ctx);err!=nil { logger.Error("pause profile jobs for inactive tenants","error",err) }
	tenantSet:=map[string]bool{}
	accountTenants.Range(func(_,value any)bool{if tenantID,ok:=value.(string);ok&&tenantID!=""{tenantSet[tenantID]=true};return true})
	for tenantID:=range tenantSet {
		jobs,err:=accountService.ClaimProfileUpdateJobs(ctx,tenantID,10)
		if err!=nil { logger.Error("claim Telegram profile update jobs","tenant_id",tenantID,"error",err);continue }
		blockedAccounts:=map[string]bool{}
		for _,job:=range jobs {
			if ctx.Err()!=nil{return}
			if blockedAccounts[job.AccountID] { continue }
			if !messageService.TenantCanOperate(ctx,job.TenantID) {
				if err:=accountService.PauseProfileUpdate(ctx,job,"tenant subscription inactive; manual review required");err!=nil {
					logger.Error("pause profile update for inactive tenant","job_id",job.ID,"error",err)
				}
				continue
			}
			if !manager.IsConnected(job.AccountID) {
				if err:=accountService.RescheduleProfileUpdate(ctx,job,30*time.Second,errors.New("Telegram account is temporarily disconnected"));err!=nil {
					logger.Error("reschedule profile update for offline account","job_id",job.ID,"error",err)
				}
				blockedAccounts[job.AccountID]=true
				continue
			}
			updateCtx,cancel:=context.WithTimeout(ctx,45*time.Second)
			var updateErr error
			if job.FirstName!="" || job.LastName!="" || job.About!="" {
				updateErr=manager.UpdateProfile(updateCtx,job.AccountID,job.FirstName,job.LastName,job.About)
			}
			if updateErr==nil && len(job.PhotoData)>0 {
				fileName:="profile.jpg"
				if len(job.PhotoData)>=8 && string(job.PhotoData[:8])=="\x89PNG\r\n\x1a\n" { fileName="profile.png" }
				updateErr=manager.UpdateProfilePhoto(updateCtx,job.AccountID,job.PhotoData,fileName)
			}
			if updateErr==nil && job.GenerateUsername {
				var username string
				for attempt:=0;attempt<8;attempt++ {
					username,updateErr=generateRandomTelegramUsername(accounts.UsernameGenerationSettings{Prefix:job.UsernamePrefix,RandomLength:job.UsernameRandomLength,Charset:job.UsernameCharset})
					if updateErr!=nil { break }
					updateErr=manager.UpdateUsername(updateCtx,job.AccountID,username)
					if updateErr==nil { job.AppliedUsername=username; break }
					if tgerr.IsUsernameOccupied(updateErr) { continue }
					break
				}
				if updateErr!=nil && tgerr.IsUsernameOccupied(updateErr) {
					updateErr=errors.New("Telegram username remained occupied after 8 random candidates")
				}
			}
			cancel()
			if updateErr!=nil {
				floodWait,_:=tgerr.AsFloodWait(updateErr)
				if floodWait>0 {
					logger.Warn("Telegram profile update FloodWait; preserving server cooldown","job_id",job.ID,"account_id",job.AccountID,"wait",floodWait.String())
					blockedAccounts[job.AccountID]=true
				}
				if err:=accountService.FailProfileUpdate(ctx,job,updateErr,floodWait);err!=nil {
					logger.Error("reschedule failed Telegram profile update","job_id",job.ID,"error",err)
				}
				logger.Warn("Telegram profile update failed","job_id",job.ID,"account_id",job.AccountID,"attempt",job.Attempts+1,"error",updateErr)
				continue
			}
			if err:=accountService.CompleteProfileUpdate(ctx,job);err!=nil {
				logger.Error("persist Telegram profile update result","job_id",job.ID,"error",err)
				continue
			}
			logger.Info("Telegram profile updated","job_id",job.ID,"account_id",job.AccountID)
		}
	}
}

func generateRandomTelegramUsername(settings accounts.UsernameGenerationSettings) (string,error) {
	const letters="abcdefghijklmnopqrstuvwxyz"
	const alphanumeric="abcdefghijklmnopqrstuvwxyz0123456789"
	settings,err:=accounts.NormalizeUsernameGenerationSettingsForWorker(settings)
	if err!=nil{return "",err}
	alphabet:=alphanumeric
	if settings.Charset=="letters" { alphabet=letters }
	randomLength:=settings.RandomLength
	random:=make([]byte,randomLength)
	if _,err:=cryptorand.Read(random);err!=nil{return "",err}
	suffix:=make([]byte,randomLength)
	for i,value:=range random { suffix[i]=alphabet[int(value)%len(alphabet)] }
	username:=settings.Prefix+string(suffix)
	if settings.Prefix=="" && username[0]>='0' && username[0]<='9' { username=string(letters[int(random[0])%len(letters)])+username[1:] }
	return username,nil
}


func newWorkerInstanceID() string {
	var raw [16]byte
	if _, err := cryptorand.Read(raw[:]); err != nil {
		return fmt.Sprintf("worker-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

func recordWorkerHeartbeat(ctx context.Context, db *sql.DB, instanceID, hostname string, processID int, running bool) error {
	status := "running"
	if !running { status = "stopped" }
	_, err := db.ExecContext(ctx, `
		INSERT INTO worker_instances(instance_id,hostname,process_id,status,started_at,heartbeat_at,stopped_at)
		VALUES($1,$2,$3,$4,NOW(),NOW(),CASE WHEN $4='stopped' THEN NOW() ELSE NULL END)
		ON CONFLICT(instance_id) DO UPDATE SET hostname=EXCLUDED.hostname,process_id=EXCLUDED.process_id,
			status=EXCLUDED.status,heartbeat_at=NOW(),
			stopped_at=CASE WHEN EXCLUDED.status='stopped' THEN NOW() ELSE NULL END
	`, instanceID, hostname, processID, status)
	return err
}
