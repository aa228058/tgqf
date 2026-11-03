package telegrammtgo

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	device "github.com/mtgo-labs/device-manager"
	"github.com/mtgo-labs/mtgo/telegram"
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
}

func NewManager(cfg RuntimeConfig) *Manager {
	if cfg.ConnectTimeout <= 0 { cfg.ConnectTimeout = 45 * time.Second }
	return &Manager{cfg: cfg, clients: make(map[string]*telegram.Client)}
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
	// Reserve the account while connecting so concurrent starts cannot create
	// duplicate MTProto connections for the same account.
	m.clients[accountID] = client
	m.mu.Unlock()

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

func (m *Manager) Stop(accountID string) error {
	m.mu.Lock()
	client := m.clients[accountID]
	delete(m.clients, accountID)
	m.mu.Unlock()
	if client == nil { return nil }
	if err := client.Disconnect(); err != nil && !errors.Is(err, telegram.ErrNotConnected) {
		return fmt.Errorf("disconnect Telegram account: %w", err)
	}
	return nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	clients := m.clients
	m.clients = make(map[string]*telegram.Client)
	m.mu.Unlock()
	var firstErr error
	for id, client := range clients {
		if err := client.Disconnect(); err != nil && !errors.Is(err, telegram.ErrNotConnected) && firstErr == nil {
			firstErr = fmt.Errorf("disconnect account %s: %w", id, err)
		}
	}
	return firstErr
}

// Keep the device-manager type visible in this package's API documentation.
var _ device.Profile
