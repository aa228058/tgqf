package accounts

import (
	"errors"
	"net/http"
	"database/sql"
	"strings"

	"github.com/aa228058/tgqf/backend/internal/auth"
)

func (h *Handler) listAccountGroups(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	accountID := strings.TrimSpace(r.PathValue("id"))
	includeLeft := r.URL.Query().Get("include_left") == "true"
	groups, state, err := h.service.ListAccountGroups(r.Context(), user.TenantID, accountID, includeLeft)
	if err != nil {
		if strings.Contains(err.Error(), "account not found") { writeError(w, http.StatusNotFound, "账号不存在"); return }
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"groups": groups, "sync": state})
}

func (h *Handler) syncAccountGroups(w http.ResponseWriter, r *http.Request) {
	if h.telegram == nil { writeError(w, http.StatusServiceUnavailable, "Telegram 连接管理器不可用"); return }
	user := auth.UserFromContext(r.Context())
	accountID := strings.TrimSpace(r.PathValue("id"))
	// Check ownership before issuing any Telegram request.
	_, _, err := h.service.ListAccountGroups(r.Context(), user.TenantID, accountID, false)
	if err != nil { writeError(w, http.StatusNotFound, "账号不存在或无权访问"); return }
	groups, err := h.telegram.ListAccountGroups(r.Context(), accountID, 1000)
	if err != nil {
		_ = h.service.MarkAccountGroupSyncFailed(r.Context(), user.TenantID, accountID, err.Error())
		writeError(w, http.StatusBadGateway, "读取 Telegram 群组失败："+err.Error())
		return
	}
	// A full 1000-item page may be truncated. Never mark unseen records as left
	// unless the snapshot is known to be complete.
	if len(groups) >= 1000 {
		message := "账号群组超过单次同步安全上限，未覆盖完整列表；旧成员关系未作离开处理"
		_ = h.service.MarkAccountGroupSyncFailed(r.Context(), user.TenantID, accountID, message)
		writeError(w, http.StatusConflict, message)
		return
	}
	if err := h.service.SaveAccountGroups(r.Context(), user.TenantID, accountID, groups); err != nil {
		writeError(w, http.StatusInternalServerError, "保存账号群组同步结果失败")
		return
	}
	saved, state, err := h.service.ListAccountGroups(r.Context(), user.TenantID, accountID, false)
	if err != nil { writeError(w, http.StatusInternalServerError, "同步已完成，但读取结果失败"); return }
	writeJSON(w, http.StatusOK, map[string]any{"groups": saved, "sync": state, "message": "账号群组同步完成"})
}

func (s *Service) MarkAccountGroupSyncFailed(ctx context.Context, tenantID, accountID, message string) error {
	if len(message) > 1000 { message = message[:1000] }
	return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO telegram_account_group_sync_state(tenant_id,account_id,status,last_started_at,last_error,updated_at)
			SELECT $1,$2,'failed',NOW(),$3,NOW()
			WHERE EXISTS(SELECT 1 FROM telegram_accounts WHERE id=$2 AND tenant_id=$1)
			ON CONFLICT(account_id) DO UPDATE SET status='failed',last_started_at=NOW(),last_error=EXCLUDED.last_error,updated_at=NOW()
		`, tenantID, accountID, message)
		return err
	})
}
