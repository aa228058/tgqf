package messages

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type Service struct { db *sql.DB }
func New(db *sql.DB) *Service { return &Service{db:db} }

type Incoming struct {
	AccountID string
	ChatID int64
	MessageID int64
	ChatType string
	SenderID int64
	ReplyToMessageID int64
	ReplyToOwnMessage bool
	IsOutgoing bool
	Text string
	MediaType string
	MessageDate time.Time
}

type Message struct {
	ID string `json:"id"`
	AccountID string `json:"account_id"`
	AccountName string `json:"account_name"`
	ChatID int64 `json:"chat_id"`
	MessageID int64 `json:"message_id"`
	ChatType string `json:"chat_type"`
	SenderID int64 `json:"sender_id"`
	ReplyToMessageID int64 `json:"reply_to_message_id,omitempty"`
	Text string `json:"text"`
	MediaType string `json:"media_type"`
	MessageDate time.Time `json:"message_date"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

type ListOptions struct { Limit int; Offset int; IncludeProcessed bool }

func ShouldCollect(chatType string, isOutgoing, replyToOwn bool) bool {
	if isOutgoing { return false }
	switch chatType {
	case "private": return true
	case "group": return replyToOwn
	default: return false
	}
}

func (s *Service) Ingest(ctx context.Context, tenantID string, in Incoming) (bool,error) {
	if tenantID=="" || !validUUID(in.AccountID) || in.ChatID==0 || in.MessageID<=0 { return false,errors.New("invalid message identity") }
	if !ShouldCollect(in.ChatType,in.IsOutgoing,in.ReplyToOwnMessage) { return false,nil }
	if len(in.Text)>16384 || len(in.MediaType)>40 { return false,errors.New("message payload too large") }
	if in.MessageDate.IsZero() { in.MessageDate=time.Now().UTC() }
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		var exists bool
		if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2)",tenantID,in.AccountID).Scan(&exists);err!=nil{return err}
		if !exists{return errors.New("account not found")}
		_,err:=tx.ExecContext(ctx,"INSERT INTO unified_messages(tenant_id,telegram_account_id,chat_id,message_id,chat_type,sender_id,reply_to_message_id,reply_to_own_message,is_outgoing,message_text,media_type,message_date) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,0),$8,FALSE,$9,$10,$11) ON CONFLICT(telegram_account_id,chat_id,message_id) DO NOTHING",
			tenantID,in.AccountID,in.ChatID,in.MessageID,in.ChatType,in.SenderID,in.ReplyToMessageID,in.ReplyToOwnMessage,strings.TrimSpace(in.Text),in.MediaType,in.MessageDate)
		return err
	})
	if err!=nil{return false,err}
	return true,nil
}

func (s *Service) List(ctx context.Context,tenantID string,opts ListOptions)([]Message,int,error){
	if opts.Limit<1||opts.Limit>100{opts.Limit=50};if opts.Offset<0{opts.Offset=0}
	out:=[]Message{};total:=0
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		filter:="tenant_id=$1";if !opts.IncludeProcessed{filter+=" AND processed_at IS NULL"}
		if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM unified_messages WHERE "+filter,tenantID).Scan(&total);err!=nil{return err}
		query:="SELECT m.id::text,m.telegram_account_id::text,COALESCE(NULLIF(a.display_name,''),a.phone,''),m.chat_id,m.message_id,m.chat_type,m.sender_id,COALESCE(m.reply_to_message_id,0),m.message_text,m.media_type,m.message_date,m.processed_at FROM unified_messages m JOIN telegram_accounts a ON a.tenant_id=m.tenant_id AND a.id=m.telegram_account_id WHERE "+strings.ReplaceAll(filter,"tenant_id","m.tenant_id")+" ORDER BY m.message_date DESC,m.id DESC LIMIT $2 OFFSET $3"
		rows,err:=tx.QueryContext(ctx,query,tenantID,opts.Limit,opts.Offset);if err!=nil{return err};defer rows.Close()
		for rows.Next(){var m Message;if err:=rows.Scan(&m.ID,&m.AccountID,&m.AccountName,&m.ChatID,&m.MessageID,&m.ChatType,&m.SenderID,&m.ReplyToMessageID,&m.Text,&m.MediaType,&m.MessageDate,&m.ProcessedAt);err!=nil{return err};out=append(out,m)}
		return rows.Err()
	})
	return out,total,err
}

func (s *Service) MarkProcessed(ctx context.Context,tenantID,messageID string)error{
	if !validUUID(messageID){return errors.New("message not found")}
	return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		res,err:=tx.ExecContext(ctx,"UPDATE unified_messages SET processed_at=COALESCE(processed_at,NOW()) WHERE tenant_id=$1 AND id=$2",tenantID,messageID)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("message not found")};return nil
	})
}

func validUUID(v string)bool {
	if len(v)!=36{return false}
	for i,c:=range v { if i==8||i==13||i==18||i==23 {if c!='-'{return false};continue};if !((c>='0'&&c<='9')||(c>='a'&&c<='f')||(c>='A'&&c<='F')){return false} }
	return true
}

func withTenantTx(ctx context.Context,db *sql.DB,tenantID string,fn func(*sql.Tx)error)error{
	if tenantID==""{return errors.New("tenant scope is required")}
	tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
	if _,err=tx.ExecContext(ctx,"SELECT set_config('app.tenant_id',$1,true)",tenantID);err!=nil{return err}
	if err=fn(tx);err!=nil{return err}
	return tx.Commit()
}
