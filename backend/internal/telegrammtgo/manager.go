package telegrammtgo

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"context"
	"strings"
	"sync"
	"time"

	"github.com/mtgo-labs/mtgo/telegram"
 "github.com/mtgo-labs/mtgo/telegram/types"
 "github.com/mtgo-labs/mtgo/telegram/params"
)

type RuntimeConfig struct {
	APIID int32
	APIHash string
	ConnectTimeout time.Duration
}

type proxyConfig struct {
	Addr string `json:"addr"`
	Username string `json:"username"`
	Password string `json:"password"`
	Protocol string `json:"protocol"`
}

// Manager owns one independent MTGo client per account. It deliberately has
// no HTTP dependencies; a dedicated worker can own this manager's lifecycle.
type Manager struct {
	mu sync.RWMutex
	cfg RuntimeConfig
	clients map[string]*telegram.Client
	pendingQR map[string]*pendingQR
 messageHandler func(accountID string, message *types.Message, ownUserID int64, replyToOwn bool)
}

// mu serializes QR polling/token refresh against cancellation and shutdown.
// The manager mutex is never held while waiting for this per-entry mutex.
type pendingQR struct {
	mu sync.Mutex
	client *telegram.Client
	token []byte
	expires int64
}
type QRChallenge struct { URL string; Expires int64 }
type QRAuthResult struct { Pending bool; Challenge *QRChallenge; SessionString string; UserID int64; Username string; FirstName string; Phone string }

func NewManager(cfg RuntimeConfig) *Manager {
	if cfg.ConnectTimeout <= 0 { cfg.ConnectTimeout = 45 * time.Second }
	return &Manager{cfg: cfg, clients: make(map[string]*telegram.Client), pendingQR: make(map[string]*pendingQR)}
}

// SetMessageHandler installs the worker-owned callback for new Telegram messages.
func (m *Manager) SetMessageHandler(handler func(accountID string, message *types.Message, ownUserID int64, replyToOwn bool)) {
	m.mu.Lock()
	m.messageHandler = handler
	m.mu.Unlock()
}

func (m *Manager) registerMessageHandler(accountID string, client *telegram.Client) {
	m.mu.RLock()
	handler := m.messageHandler
	m.mu.RUnlock()
	if handler == nil { return }
	client.OnMessage(func(ctx *telegram.Context, message *types.Message) {
		if message == nil { return }
		me := client.Me()
		if me == nil { return }
		replyToOwn := false
		if message.ReplyToID > 0 && message.ChatID != 0 {
			lookupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			replied, err := client.GetMessages(lookupCtx, message.ChatID, []int32{message.ReplyToID})
			cancel()
			if err == nil && len(replied) > 0 && replied[0] != nil {
				replyToOwn = replied[0].Out || replied[0].FromID == me.ID
			}
		}
		handler(accountID, message, me.ID, replyToOwn)
	})
}

// Start restores an existing MTGo-compatible session and connects it.
// The caller must pass decrypted session data only in memory.
func (m *Manager) Start(accountID, sessionString, proxyJSON string) error {
	accountID = strings.TrimSpace(accountID)
	sessionString = strings.TrimSpace(sessionString)
	if accountID == "" { return errors.New("account ID is required") }
	if sessionString == "" { return errors.New("session string is required") }

	var proxy *telegram.Proxy
	if strings.TrimSpace(proxyJSON) != "" && strings.TrimSpace(proxyJSON) != "null" && strings.TrimSpace(proxyJSON) != "{}" {
		var p proxyConfig
		if err := json.Unmarshal([]byte(proxyJSON), &p); err != nil { return errors.New("invalid proxy configuration") }
		p.Addr = strings.TrimSpace(p.Addr)
		if p.Addr == "" { return errors.New("proxy address is required") }
		protocol := strings.ToLower(strings.TrimSpace(p.Protocol))
		switch protocol { case "", "socks5", "socks4", "http", "https": default: return fmt.Errorf("unsupported proxy protocol %q", protocol) }
		proxy = &telegram.Proxy{Addr:p.Addr, Username:p.Username, Password:p.Password, Protocol:protocol}
	}

	profile, err := DeviceProfile(accountID, "", "", "", "")
	if err != nil { return err }
	cfg := telegram.Config{SessionString:sessionString, SessionName:"tgqf-"+accountID, InMemory:true, Proxy:proxy}
	ApplyDeviceProfile(&cfg, profile)
	client, err := telegram.NewClient(m.cfg.APIID, m.cfg.APIHash, &cfg)
	if err != nil { return fmt.Errorf("create MTGo client: %w", err) }

	m.mu.Lock()
	if _, exists := m.clients[accountID]; exists {
		m.mu.Unlock()
		_ = client.Disconnect()
		return errors.New("account client is already managed")
	}
	m.clients[accountID] = client
	m.mu.Unlock()
	m.registerMessageHandler(accountID, client)

	if err := client.Connect(m.cfg.ConnectTimeout); err != nil {
		m.mu.Lock()
		if m.clients[accountID] == client { delete(m.clients, accountID) }
		m.mu.Unlock()
		_ = client.Disconnect()
		return fmt.Errorf("connect Telegram account: %w", err)
	}
	if client.Me() == nil {
		m.mu.Lock()
		if m.clients[accountID] == client { delete(m.clients, accountID) }
		m.mu.Unlock()
		_ = client.Disconnect()
		return errors.New("Telegram session connected but is not authorized")
	}
	return nil
}

func (m *Manager) IsConnected(accountID string) bool {
	m.mu.RLock()
	client := m.clients[accountID]
	m.mu.RUnlock()
	return client != nil && client.IsConnected()
}

// ExportSession returns the latest session representation for encrypted
// persistence by the worker's repository layer.
func (m *Manager) ExportSession(accountID string) (string, error) {
	m.mu.RLock()
	client := m.clients[accountID]
	m.mu.RUnlock()
	if client == nil { return "", errors.New("account client is not managed") }
	if !client.IsConnected() { return "", errors.New("account client is disconnected") }
	value, err := client.ExportSessionString()
	if err != nil { return "", fmt.Errorf("export Telegram session: %w", err) }
	return value, nil
}

func disconnectPending(entry *pendingQR) error {
	if entry == nil { return nil }
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.client == nil { return nil }
	if err := entry.client.Disconnect(); err != nil && !errors.Is(err, telegram.ErrNotConnected) {
		return fmt.Errorf("disconnect pending QR client: %w", err)
	}
	return nil
}

func (m *Manager) Stop(accountID string) error {
	m.mu.Lock()
	client := m.clients[accountID]
	pending := m.pendingQR[accountID]
	delete(m.clients, accountID)
	delete(m.pendingQR, accountID)
	m.mu.Unlock()
	if err := disconnectPending(pending); err != nil { return err }
	if client == nil { return nil }
	if err := client.Disconnect(); err != nil && !errors.Is(err, telegram.ErrNotConnected) {
		return fmt.Errorf("disconnect Telegram account: %w", err)
	}
	return nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	clients := m.clients
	pending := m.pendingQR
	m.clients = make(map[string]*telegram.Client)
	m.pendingQR = make(map[string]*pendingQR)
	m.mu.Unlock()
	var firstErr error
	for id, entry := range pending {
		if err := disconnectPending(entry); err != nil && firstErr == nil { firstErr = fmt.Errorf("disconnect pending QR account %s: %w", id, err) }
	}
	for id, client := range clients {
		if err := client.Disconnect(); err != nil && !errors.Is(err, telegram.ErrNotConnected) && firstErr == nil {
			firstErr = fmt.Errorf("disconnect account %s: %w", id, err)
		}
	}
	return firstErr
}

func (m *Manager) StartQR(ctx context.Context, accountID, proxyJSON string) (QRAuthResult, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" { return QRAuthResult{}, errors.New("account ID is required") }
	m.mu.RLock()
	_, active := m.clients[accountID]
	_, pending := m.pendingQR[accountID]
	m.mu.RUnlock()
	if active || pending { return QRAuthResult{}, errors.New("account already has an active or pending client") }
	var proxy *telegram.Proxy
	if strings.TrimSpace(proxyJSON) != "" && strings.TrimSpace(proxyJSON) != "null" && strings.TrimSpace(proxyJSON) != "{}" {
		var p proxyConfig
		if err := json.Unmarshal([]byte(proxyJSON), &p); err != nil { return QRAuthResult{}, errors.New("invalid proxy configuration") }
		p.Addr = strings.TrimSpace(p.Addr)
		if p.Addr == "" { return QRAuthResult{}, errors.New("proxy address is required") }
		protocol := strings.ToLower(strings.TrimSpace(p.Protocol))
		switch protocol { case "", "socks5", "socks4", "http", "https": default: return QRAuthResult{}, fmt.Errorf("unsupported proxy protocol %q", protocol) }
		proxy = &telegram.Proxy{Addr:p.Addr, Username:p.Username, Password:p.Password, Protocol:protocol}
	}
	profile, err := DeviceProfile(accountID, "", "", "", "")
	if err != nil { return QRAuthResult{}, err }
	cfg := telegram.Config{SessionName:"tgqf-"+accountID, InMemory:true, Proxy:proxy}
	ApplyDeviceProfile(&cfg, profile)
	client, err := telegram.NewClient(m.cfg.APIID, m.cfg.APIHash, &cfg)
	if err != nil { return QRAuthResult{}, fmt.Errorf("create MTGo QR client: %w", err) }
	if err := client.Connect(m.cfg.ConnectTimeout); err != nil { _ = client.Disconnect(); return QRAuthResult{}, fmt.Errorf("connect MTGo QR client: %w", err) }
	token, err := client.GetQRCodeLoginToken(ctx)
	if err != nil { _ = client.Disconnect(); return QRAuthResult{}, fmt.Errorf("request Telegram QR token: %w", err) }
	if token.User != nil {
		sessionString, exportErr := client.ExportSessionString()
		if exportErr != nil { _ = client.Disconnect(); return QRAuthResult{}, fmt.Errorf("export immediately authorized QR session: %w", exportErr) }
		m.mu.Lock()
		if _, exists := m.clients[accountID]; exists { m.mu.Unlock(); _ = client.Disconnect(); return QRAuthResult{}, errors.New("account client is already managed") }
		if _, exists := m.pendingQR[accountID]; exists { m.mu.Unlock(); _ = client.Disconnect(); return QRAuthResult{}, errors.New("account QR login is already pending") }
		m.clients[accountID] = client
		m.mu.Unlock()
		return QRAuthResult{SessionString:sessionString, UserID:token.User.ID, Username:token.User.Username, FirstName:token.User.FirstName, Phone:token.User.Phone}, nil
	}
	entry := &pendingQR{client:client, token:append([]byte(nil), token.Token...), expires:int64(token.Expires)}
	m.mu.Lock()
	if _, exists := m.clients[accountID]; exists { m.mu.Unlock(); _ = client.Disconnect(); return QRAuthResult{}, errors.New("account client is already managed") }
	if _, exists := m.pendingQR[accountID]; exists { m.mu.Unlock(); _ = client.Disconnect(); return QRAuthResult{}, errors.New("account QR login is already pending") }
	m.pendingQR[accountID] = entry
	m.mu.Unlock()
	return QRAuthResult{Pending:true,Challenge:&QRChallenge{URL:"tg://login?token="+base64.RawURLEncoding.EncodeToString(token.Token),Expires:int64(token.Expires)}},nil
}

func (m *Manager) PollQR(ctx context.Context, accountID string) (QRAuthResult, error) {
	m.mu.RLock()
	entry := m.pendingQR[accountID]
	m.mu.RUnlock()
	if entry == nil { return QRAuthResult{}, errors.New("no pending QR login for account") }

	entry.mu.Lock()
	defer entry.mu.Unlock()
	// Re-check map membership after acquiring the entry lock: cancellation may
	// have removed this challenge while this poll was waiting.
	m.mu.RLock()
	current := m.pendingQR[accountID]
	m.mu.RUnlock()
	if current != entry { return QRAuthResult{}, errors.New("QR login was cancelled") }

	user, err := entry.client.CheckQRCodeLoginToken(ctx, entry.token)
	if err != nil {
		message := strings.ToLower(err.Error())
		if strings.Contains(message, "not yet scanned") {
			if time.Now().Unix() >= entry.expires {
				refreshed, refreshErr := entry.client.GetQRCodeLoginToken(ctx)
				if refreshErr != nil { return QRAuthResult{}, fmt.Errorf("refresh expired QR token: %w", refreshErr) }
				if refreshed.User != nil { user = refreshed.User } else {
					entry.token = append(entry.token[:0], refreshed.Token...)
					entry.expires = int64(refreshed.Expires)
					return QRAuthResult{Pending:true,Challenge:&QRChallenge{URL:"tg://login?token="+base64.RawURLEncoding.EncodeToString(refreshed.Token),Expires:int64(refreshed.Expires)}},nil
				}
			} else {
				return QRAuthResult{Pending:true,Challenge:&QRChallenge{URL:"tg://login?token="+base64.RawURLEncoding.EncodeToString(entry.token),Expires:entry.expires}},nil
			}
		} else { return QRAuthResult{}, fmt.Errorf("check Telegram QR login: %w", err) }
	}
	if user == nil { return QRAuthResult{}, errors.New("Telegram QR login returned no user") }
	sessionString, err := entry.client.ExportSessionString()
	if err != nil { return QRAuthResult{}, fmt.Errorf("export QR-authorized session: %w", err) }

	m.mu.Lock()
	if m.pendingQR[accountID] != entry { m.mu.Unlock(); return QRAuthResult{}, errors.New("QR login was cancelled") }
	delete(m.pendingQR, accountID)
	m.clients[accountID] = entry.client
	m.mu.Unlock()
	return QRAuthResult{SessionString:sessionString,UserID:user.ID,Username:user.Username,FirstName:user.FirstName,Phone:user.Phone},nil
}

func (m *Manager) CancelQR(accountID string) error {
	m.mu.Lock()
	entry := m.pendingQR[accountID]
	delete(m.pendingQR, accountID)
	m.mu.Unlock()
	return disconnectPending(entry)
}

// ManagedAccountIDs returns a snapshot of accounts currently owned by Manager.
func (m *Manager) ManagedAccountIDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]string, 0, len(m.clients))
	for id := range m.clients { ids = append(ids, id) }
	return ids
}

func (m *Manager) SendReply(ctx context.Context, accountID string, chatID int64, replyToMessageID int64, textBody string) (int64,error) {
	if replyToMessageID<=0 || replyToMessageID>int64(^uint32(0)>>1) {
		return 0,errors.New("invalid Telegram reply message id")
	}
	m.mu.RLock()
	client:=m.clients[accountID]
	m.mu.RUnlock()
	if client==nil || !client.IsConnected() {
		return 0,errors.New("Telegram account is not connected")
	}
	sent,err:=client.SendMessage(ctx,chatID,textBody,&params.SendMessage{ReplyToMessageID:int32(replyToMessageID)})
	if err!=nil{return 0,fmt.Errorf("send Telegram reply: %w",err)}
	if sent==nil || sent.ID<=0{return 0,errors.New("Telegram returned an empty sent message")}
	return int64(sent.ID),nil
}


// GetChatHistory returns a bounded recent history slice for a known chat.
func (m *Manager) GetChatHistory(ctx context.Context, accountID string, chatID int64, limit int) ([]*types.Message, error) {
	m.mu.RLock()
	client := m.clients[accountID]
	m.mu.RUnlock()
	if client == nil || !client.IsConnected() { return nil, errors.New("Telegram account is not connected") }
	if limit < 1 || limit > 100 { limit = 50 }
	return client.GetChatHistory(ctx, chatID, limit, 0)
}

// IsRepliesToOwnMessages verifies a batch of historical reply targets with one
// Telegram lookup. Missing targets are intentionally treated as non-matches.
func (m *Manager) IsRepliesToOwnMessages(ctx context.Context, accountID string, chatID int64, replyToIDs []int32) map[int32]bool {
	matched := make(map[int32]bool)
	if len(replyToIDs) == 0 { return matched }
	m.mu.RLock()
	client := m.clients[accountID]
	m.mu.RUnlock()
	if client == nil || !client.IsConnected() { return matched }
	me := client.Me()
	if me == nil { return matched }
	unique := make([]int32, 0, len(replyToIDs))
	seen := make(map[int32]struct{}, len(replyToIDs))
	for _, id := range replyToIDs {
		if id <= 0 { continue }
		if _, ok := seen[id]; ok { continue }
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	if len(unique) == 0 { return matched }
	originals, err := client.GetMessages(ctx, chatID, unique)
	if err != nil { return matched }
	for _, original := range originals {
		if original != nil && original.ID > 0 && (original.Out || original.FromID == me.ID) {
			matched[original.ID] = true
		}
	}
	return matched
}

// IsReplyToOwnMessage verifies the historical reply target against the
// authenticated account. A failed lookup is treated as not matching.
func (m *Manager) IsReplyToOwnMessage(ctx context.Context, accountID string, chatID int64, replyToID int32) bool {
	if replyToID <= 0 { return false }
	m.mu.RLock()
	client := m.clients[accountID]
	m.mu.RUnlock()
	if client == nil || !client.IsConnected() { return false }
	me := client.Me()
	if me == nil { return false }
	original, err := client.GetMessages(ctx, chatID, []int32{replyToID})
	if err != nil || len(original) == 0 || original[0] == nil { return false }
	return original[0].Out || original[0].FromID == me.ID
}
