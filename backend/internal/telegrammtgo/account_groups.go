package telegrammtgo

import (
	"context"
	"errors"
	"fmt"

	"github.com/mtgo-labs/mtgo/telegram/types"
)

// AccountGroup is a snapshot of a group/channel visible in an account's
// Telegram dialogs. It contains no session credentials or private message data.
type AccountGroup struct {
	ChatID       int64  `json:"chat_id"`
	ChatType     string `json:"chat_type"`
	Title        string `json:"title"`
	Username     string `json:"username"`
	MembersCount int    `json:"members_count"`
	IsAdmin      bool   `json:"is_admin"`
	IsCreator    bool   `json:"is_creator"`
}

// ListAccountGroups reads the account's current group dialogs without joining,
// leaving, or otherwise changing Telegram membership. Telegram dialog results
// are bounded by limit to keep a manual sync from issuing unbounded requests.
func (m *Manager) ListAccountGroups(ctx context.Context, accountID string, limit int) ([]AccountGroup, error) {
	if limit < 1 || limit > 1000 {
		return nil, errors.New("group sync limit must be between 1 and 1000")
	}
	m.mu.RLock()
	client := m.clients[accountID]
	m.mu.RUnlock()
	if client == nil || !client.IsConnected() {
		return nil, errors.New("Telegram account is not connected")
	}
	chats, err := client.GetDialogs(ctx, limit, 0)
	if err != nil {
		return nil, fmt.Errorf("list Telegram dialogs: %w", err)
	}
	out := make([]AccountGroup, 0, len(chats))
	for _, chat := range chats {
		if chat == nil || chat.ID == 0 || chat.IsLeft || chat.IsBanned {
			continue
		}
		kind := ""
		switch chat.Type {
		case types.ChatTypeGroup:
			kind = "group"
		case types.ChatTypeSupergroup, types.ChatTypeForum:
			kind = "supergroup"
		case types.ChatTypeChannel:
			kind = "channel"
		default:
			continue
		}
		out = append(out, AccountGroup{
			ChatID: chat.ID, ChatType: kind, Title: chat.Title,
			Username: chat.Username, MembersCount: chat.MembersCount,
			IsAdmin: chat.IsAdmin, IsCreator: chat.IsCreator,
		})
	}
	return out, nil
}
