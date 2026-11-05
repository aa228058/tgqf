package messages

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"database/sql"
	"fmt"
)

// isAmbiguousTelegramSendError identifies failures where the client may have
// lost the response after Telegram accepted the send. Replaying those actions
// automatically can create duplicate messages, so callers must quarantine them
// for manual verification instead of consuming the retry budget.
func isAmbiguousTelegramSendError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
		return true
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return true
	}
	upper := strings.ToUpper(err.Error())
	for _, marker := range []string{
		"SEND OUTCOME UNCERTAIN",
		"UNEXPECTED EOF",
		"CONNECTION RESET",
		"CONNECTION ABORTED",
		"BROKEN PIPE",
		"I/O TIMEOUT",
		"TLS HANDSHAKE TIMEOUT",
		"TRANSPORT IS CLOSING",
		"CLIENT IS CLOSED",
	} {
		if strings.Contains(upper, marker) {
			return true
		}
	}
	return false
}


type telegramCapabilityIssue struct {
	capability string
	status string
	chatID int64
	rpcError string
	reason string
}

// classifyTelegramCapabilityIssue is deliberately conservative: it records
// only explicit Telegram permission/flood errors, never generic transport or
// business failures. Chat-scoped errors stay scoped to that chat.
func classifyTelegramCapabilityIssue(err error, floodWait bool, chatType string, chatID int64) *telegramCapabilityIssue {
	if err == nil { return nil }
	upper := strings.ToUpper(err.Error())
	issue := &telegramCapabilityIssue{status:"restricted", chatID:chatID, rpcError:upper, reason:"Telegram 明确拒绝了该操作"}
	switch {
	case strings.Contains(upper,"PEER_FLOOD"):
		issue.capability="private_message"
		issue.chatID=0
		issue.reason="Telegram 返回 PEER_FLOOD：账号的主动私信能力可能受到限制"
	case strings.Contains(upper,"CHAT_SEND_MEDIA_FORBIDDEN"):
		issue.capability="media_send"
		issue.reason="该会话禁止此账号发送媒体"
	case strings.Contains(upper,"USER_BANNED_IN_CHANNEL"):
		issue.capability="group_message"
		issue.reason="该群组或频道拒绝此账号发言；不代表账号在其他群组也受限"
	case strings.Contains(upper,"CHAT_WRITE_FORBIDDEN"), strings.Contains(upper,"CHAT_SEND_PLAIN_FORBIDDEN"), strings.Contains(upper,"CHAT_RESTRICTED"):
		if chatType=="group" || chatType=="channel" {
			issue.capability="group_message"
			issue.reason="该群组或频道拒绝此账号发言；不代表账号在其他群组也受限"
		} else {
			issue.capability="message_send"
			issue.reason="Telegram 拒绝了此目标会话的消息发送；目标类型未知或不是群组，不能据此判定账号整体受限"
		}
	case floodWait:
		issue.capability="message_send"
		if chatType=="group" || chatType=="channel" { issue.capability="group_message" }
		if chatType=="private" { issue.capability="private_message"; issue.chatID=0 }
		issue.status="cooldown"
		issue.reason="Telegram 要求等待 FloodWait 冷却结束"
	default:
		return nil
	}
	if issue.rpcError=="" { issue.rpcError=fmt.Sprint(err) }
	if len(issue.rpcError)>300 { issue.rpcError=issue.rpcError[:300] }
	return issue
}

// recordTelegramCapabilityIssueTx stores an observed restriction without
// inferring that other chats or features are restricted.
func recordTelegramCapabilityIssueTx(tx *sql.Tx, tenantID, accountID string, chatID int64, err error, floodWait bool) error {
	chatType := ""
	if chatID != 0 {
		_ = tx.QueryRow(`SELECT chat_type FROM tenant_message_chats WHERE tenant_id=$1 AND telegram_account_id=$2 AND chat_id=$3`,tenantID,accountID,chatID).Scan(&chatType)
	}
	issue := classifyTelegramCapabilityIssue(err,floodWait,chatType,chatID)
	if issue==nil { return nil }
	_,dbErr:=tx.Exec(`INSERT INTO telegram_account_capability_status
		(tenant_id,account_id,capability,chat_id,status,rpc_error,reason,source,last_observed_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,'message_reply',NOW())
		ON CONFLICT(tenant_id,account_id,capability,chat_id)
		DO UPDATE SET status=EXCLUDED.status,rpc_error=EXCLUDED.rpc_error,reason=EXCLUDED.reason,source=EXCLUDED.source,last_observed_at=NOW()`,
		tenantID,accountID,issue.capability,issue.chatID,issue.status,issue.rpcError,issue.reason)
	return dbErr
}


func recordJoinCapabilityIssueTx(tx *sql.Tx,tenantID,accountID,reason string,floodWait bool) error {
	upper:=strings.ToUpper(reason)
	status:="restricted"
	message:="Telegram 明确拒绝了该账号的加群操作"
	if floodWait || strings.Contains(upper,"FLOOD_WAIT") {
		status="cooldown"
		message="Telegram 要求等待加群 FloodWait 冷却结束"
	} else if strings.Contains(upper,"USER_RESTRICTED") {
		message="Telegram 返回 USER_RESTRICTED：账号当前加入群组的能力受到限制"
	} else {
		return nil
	}
	rpc:=upper;if len(rpc)>300{rpc=rpc[:300]}
	_,err:=tx.Exec(`INSERT INTO telegram_account_capability_status
		(tenant_id,account_id,capability,chat_id,status,rpc_error,reason,source,last_observed_at)
		VALUES($1,$2,'group_join',0,$3,$4,$5,'group_join_task',NOW())
		ON CONFLICT(tenant_id,account_id,capability,chat_id)
		DO UPDATE SET status=EXCLUDED.status,rpc_error=EXCLUDED.rpc_error,reason=EXCLUDED.reason,source=EXCLUDED.source,last_observed_at=NOW()`,
		tenantID,accountID,status,rpc,message)
	return err
}
