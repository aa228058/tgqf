package messages

import (
	"context"
	"database/sql"
	"errors"
	"encoding/json"
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
	ChatTitle string
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
	ChatTitle string `json:"chat_title"`
	SenderID int64 `json:"sender_id"`
	ReplyToMessageID int64 `json:"reply_to_message_id,omitempty"`
	Text string `json:"text"`
	MediaType string `json:"media_type"`
	MessageDate time.Time `json:"message_date"`
	ProcessedAt *time.Time `json:"processed_at,omitempty"`
}

type ListOptions struct { Limit int; Offset int; IncludeProcessed bool }

type CollectionSettings struct {
	Enabled bool `json:"enabled"`
	IncludeAllAccounts bool `json:"include_all_accounts"`
	AccountIDs []string `json:"account_ids"`
	ChatIDs []int64 `json:"chat_ids"`
	IncludeText bool `json:"include_text"`
	IncludeMedia bool `json:"include_media"`
	Keywords []string `json:"keywords"`
}

func DefaultCollectionSettings() CollectionSettings {
	return CollectionSettings{Enabled:true,IncludeAllAccounts:true,AccountIDs:[]string{},ChatIDs:[]int64{},IncludeText:true,IncludeMedia:true,Keywords:[]string{}}
}

func (s CollectionSettings) Allows(in Incoming) bool {
	if !s.Enabled { return false }
	if !s.IncludeAllAccounts {
		found:=false
		for _,id:=range s.AccountIDs { if id==in.AccountID { found=true;break } }
		if !found { return false }
	}
	if len(s.ChatIDs)>0 {
		found:=false
		for _,id:=range s.ChatIDs { if id==in.ChatID { found=true;break } }
		if !found { return false }
	}
	textPresent:=strings.TrimSpace(in.Text)!=""
	mediaPresent:=strings.TrimSpace(in.MediaType)!=""
	if !(textPresent&&s.IncludeText) && !(mediaPresent&&s.IncludeMedia) { return false }
	if len(s.Keywords)>0 {
		if !textPresent { return false }
		lower:=strings.ToLower(in.Text)
		matched:=false
		for _,keyword:=range s.Keywords { if strings.Contains(lower,strings.ToLower(keyword)) { matched=true;break } }
		if !matched { return false }
	}
	return true
}

func validateSettings(settings CollectionSettings) (CollectionSettings,error) {
	if settings.AccountIDs==nil { settings.AccountIDs=[]string{} }
	if settings.ChatIDs==nil { settings.ChatIDs=[]int64{} }
	if settings.Keywords==nil { settings.Keywords=[]string{} }
	if len(settings.AccountIDs)>100 || len(settings.ChatIDs)>1000 || len(settings.Keywords)>50 { return settings,errors.New("message filter limits exceeded") }
	seen:=map[string]bool{}
	for _,id:=range settings.AccountIDs { if !validUUID(id)||seen[id] { return settings,errors.New("invalid or duplicate account id in filters") };seen[id]=true }
	seenChats:=map[int64]bool{}
	for _,id:=range settings.ChatIDs { if id==0||seenChats[id] { return settings,errors.New("invalid or duplicate chat id in filters") };seenChats[id]=true }
	for i,k:=range settings.Keywords { k=strings.TrimSpace(k);if k==""||len(k)>64{return settings,errors.New("keywords must contain 1-64 characters")};settings.Keywords[i]=k }
	return settings,nil
}

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
	collected:=false
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		var exists bool
		if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2)",tenantID,in.AccountID).Scan(&exists);err!=nil{return err}
		if !exists{return errors.New("account not found")}
		settings,err:=loadSettings(ctx,tx,tenantID);if err!=nil{return err}
		if !settings.Allows(in){return nil}
		_,err=tx.ExecContext(ctx,"INSERT INTO unified_messages(tenant_id,telegram_account_id,chat_id,message_id,chat_type,chat_title,sender_id,reply_to_message_id,reply_to_own_message,is_outgoing,message_text,media_type,message_date) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,0),$9,FALSE,$10,$11,$12) ON CONFLICT(telegram_account_id,chat_id,message_id) DO NOTHING",
			tenantID,in.AccountID,in.ChatID,in.MessageID,in.ChatType,strings.TrimSpace(in.ChatTitle),in.SenderID,in.ReplyToMessageID,in.ReplyToOwnMessage,strings.TrimSpace(in.Text),in.MediaType,in.MessageDate)
		if err==nil { collected=true }
		return err
	})
	if err!=nil{return false,err}
	return collected,nil
}

func (s *Service) List(ctx context.Context,tenantID string,opts ListOptions)([]Message,int,error){
	if opts.Limit<1||opts.Limit>100{opts.Limit=50};if opts.Offset<0{opts.Offset=0}
	out:=[]Message{};total:=0
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		filter:="tenant_id=$1";if !opts.IncludeProcessed{filter+=" AND processed_at IS NULL"}
		if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM unified_messages WHERE "+filter,tenantID).Scan(&total);err!=nil{return err}
		query:="SELECT m.id::text,m.telegram_account_id::text,COALESCE(NULLIF(a.display_name,''),a.phone,''),m.chat_id,m.message_id,m.chat_type,m.chat_title,m.sender_id,COALESCE(m.reply_to_message_id,0),m.message_text,m.media_type,m.message_date,m.processed_at FROM unified_messages m JOIN telegram_accounts a ON a.tenant_id=m.tenant_id AND a.id=m.telegram_account_id WHERE "+strings.ReplaceAll(filter,"tenant_id","m.tenant_id")+" ORDER BY m.message_date DESC,m.id DESC LIMIT $2 OFFSET $3"
		rows,err:=tx.QueryContext(ctx,query,tenantID,opts.Limit,opts.Offset);if err!=nil{return err};defer rows.Close()
		for rows.Next(){var m Message;if err:=rows.Scan(&m.ID,&m.AccountID,&m.AccountName,&m.ChatID,&m.MessageID,&m.ChatType,&m.ChatTitle,&m.SenderID,&m.ReplyToMessageID,&m.Text,&m.MediaType,&m.MessageDate,&m.ProcessedAt);err!=nil{return err};out=append(out,m)}
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

func loadSettings(ctx context.Context,tx *sql.Tx,tenantID string)(CollectionSettings,error){
	settings:=DefaultCollectionSettings()
	var raw []byte
	err:=tx.QueryRowContext(ctx,"SELECT settings FROM tenant_message_settings WHERE tenant_id=$1",tenantID).Scan(&raw)
	if errors.Is(err,sql.ErrNoRows){return settings,nil}
	if err!=nil{return settings,err}
	if err=json.Unmarshal(raw,&settings);err!=nil{return settings,err}
	return settings,nil
}

func (s *Service) GetSettings(ctx context.Context,tenantID string)(CollectionSettings,error){
	settings:=DefaultCollectionSettings()
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{var err error;settings,err=loadSettings(ctx,tx,tenantID);return err})
	return settings,err
}

func (s *Service) UpdateSettings(ctx context.Context,tenantID string,settings CollectionSettings)(CollectionSettings,error){
	settings,err:=validateSettings(settings);if err!=nil{return settings,err}
	raw,err:=json.Marshal(settings);if err!=nil{return settings,err}
	err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		for _,accountID:=range settings.AccountIDs {
			var exists bool
			if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2)",tenantID,accountID).Scan(&exists);err!=nil{return err}
			if !exists{return errors.New("filter account does not belong to tenant")}
		}
		_,err:=tx.ExecContext(ctx,"INSERT INTO tenant_message_settings(tenant_id,settings,updated_at) VALUES($1,$2::jsonb,NOW()) ON CONFLICT(tenant_id) DO UPDATE SET settings=EXCLUDED.settings,updated_at=NOW()",tenantID,string(raw))
		return err
	})
	return settings,err
}

type ChatSummary struct {
	AccountID string `json:"account_id"`
	AccountName string `json:"account_name"`
	ChatID int64 `json:"chat_id"`
	ChatTitle string `json:"chat_title"`
	ChatType string `json:"chat_type"`
	UnprocessedCount int `json:"unprocessed_count"`
	LastMessage string `json:"last_message"`
	LastMessageDate time.Time `json:"last_message_date"`
}

func (s *Service) ListChats(ctx context.Context,tenantID string,limit,offset int)([]ChatSummary,int,error){
	if limit<1||limit>100{limit=50};if offset<0{offset=0}
	out:=[]ChatSummary{};total:=0
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		if err:=tx.QueryRowContext(ctx,"SELECT COUNT(DISTINCT (telegram_account_id,chat_id)) FROM unified_messages WHERE tenant_id=$1",tenantID).Scan(&total);err!=nil{return err}
		query:="SELECT g.telegram_account_id::text,COALESCE(NULLIF(a.display_name,''),a.phone,''),g.chat_id,g.chat_title,g.chat_type,g.unprocessed_count,COALESCE(latest.message_text,''),g.last_message_date FROM (SELECT telegram_account_id,chat_id,MAX(chat_title) AS chat_title,MAX(chat_type) AS chat_type,COUNT(*) FILTER (WHERE processed_at IS NULL) AS unprocessed_count,MAX(message_date) AS last_message_date FROM unified_messages WHERE tenant_id=$1 GROUP BY telegram_account_id,chat_id) g JOIN telegram_accounts a ON a.tenant_id=$1 AND a.id=g.telegram_account_id LEFT JOIN LATERAL (SELECT message_text FROM unified_messages m2 WHERE m2.tenant_id=$1 AND m2.telegram_account_id=g.telegram_account_id AND m2.chat_id=g.chat_id ORDER BY m2.message_date DESC,m2.id DESC LIMIT 1) latest ON TRUE ORDER BY g.last_message_date DESC LIMIT $2 OFFSET $3"
		rows,err:=tx.QueryContext(ctx,query,tenantID,limit,offset);if err!=nil{return err};defer rows.Close()
		for rows.Next(){var item ChatSummary;if err:=rows.Scan(&item.AccountID,&item.AccountName,&item.ChatID,&item.ChatTitle,&item.ChatType,&item.UnprocessedCount,&item.LastMessage,&item.LastMessageDate);err!=nil{return err};out=append(out,item)}
		return rows.Err()
	})
	return out,total,err
}


var ErrNoReplyJob = errors.New("no pending reply job")

type ReplyJob struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	AccountID string `json:"account_id"`
	ChatID int64 `json:"chat_id"`
	ReplyToMessageID int64 `json:"reply_to_message_id"`
	Text string `json:"text"`
	Attempts int `json:"attempts"`
	Status string `json:"status"`
	TelegramMessageID int64 `json:"telegram_message_id,omitempty"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Service) CreateReply(ctx context.Context, tenantID, messageID, textBody string) (ReplyJob,error) {
	textBody=strings.TrimSpace(textBody)
	if !validUUID(messageID) || textBody=="" || len([]rune(textBody))>4096 {
		return ReplyJob{},errors.New("reply text must contain 1-4096 characters and message id must be valid")
	}
	var job ReplyJob
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		err:=tx.QueryRowContext(ctx,`SELECT telegram_account_id::text,chat_id,message_id FROM unified_messages WHERE tenant_id=$1 AND id=$2`,tenantID,messageID).Scan(&job.AccountID,&job.ChatID,&job.ReplyToMessageID)
		if errors.Is(err,sql.ErrNoRows){return errors.New("message not found")};if err!=nil{return err}
		err=tx.QueryRowContext(ctx,`INSERT INTO message_reply_jobs(tenant_id,unified_message_id,telegram_account_id,chat_id,reply_to_message_id,message_text) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text,status,attempts,created_at`,tenantID,messageID,job.AccountID,job.ChatID,job.ReplyToMessageID,textBody).Scan(&job.ID,&job.Status,&job.Attempts,&job.CreatedAt)
		if err==nil {job.TenantID=tenantID;job.Text=textBody}
		return err
	})
	return job,err
}

// ClaimPendingReplies claims due jobs under each tenant's RLS scope. Expired
// processing leases are reclaimable after a worker restart.
func (s *Service) ClaimPendingReplies(ctx context.Context, maxJobs int) ([]ReplyJob,error) {
	if maxJobs<1||maxJobs>50 {maxJobs=10}
	rows,err:=s.db.QueryContext(ctx,`SELECT id::text FROM tenants WHERE status='active' AND expires_at>NOW()`)
	if err!=nil{return nil,err}
	var tenants []string
	for rows.Next(){var id string;if err:=rows.Scan(&id);err!=nil{rows.Close();return nil,err};tenants=append(tenants,id)}
	if err:=rows.Err();err!=nil{rows.Close();return nil,err};rows.Close()
	jobs:=make([]ReplyJob,0,maxJobs)
	for _,tenantID:=range tenants {
		if len(jobs)>=maxJobs {break}
		err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
			remaining:=maxJobs-len(jobs)
			if remaining>2 { remaining=2 }
			rows,err:=tx.QueryContext(ctx,`WITH picked AS (
				SELECT id FROM message_reply_jobs
				WHERE tenant_id=$1 AND ((status='pending' AND available_at<=NOW()) OR (status='processing' AND lease_until<NOW()))
				ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT $2
			)
			UPDATE message_reply_jobs j SET status='processing',attempts=j.attempts+1,lease_until=NOW()+INTERVAL '5 minutes',updated_at=NOW()
			FROM picked WHERE j.id=picked.id
			RETURNING j.id::text,j.tenant_id::text,j.telegram_account_id::text,j.chat_id,j.reply_to_message_id,j.message_text,j.attempts,j.status,j.created_at`,tenantID,remaining)
			if err!=nil{return err}
			defer rows.Close()
			for rows.Next(){var job ReplyJob;if err:=rows.Scan(&job.ID,&job.TenantID,&job.AccountID,&job.ChatID,&job.ReplyToMessageID,&job.Text,&job.Attempts,&job.Status,&job.CreatedAt);err!=nil{return err};jobs=append(jobs,job)}
			return rows.Err()
		})
		if err!=nil{return nil,err}
	}
	return jobs,nil
}

func (s *Service) CompleteReply(ctx context.Context, job ReplyJob, telegramMessageID int64) error {
	if telegramMessageID<=0{return errors.New("invalid Telegram message id")}
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		res,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status='sent',telegram_message_id=$3,last_error='',lease_until=NULL,updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='processing'`,job.TenantID,job.ID,telegramMessageID)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("reply job lease not found")};return nil
	})
}

func (s *Service) FailReply(ctx context.Context, job ReplyJob, cause error) error {
	if cause==nil{cause=errors.New("unknown Telegram send error")}
	message:=cause.Error();if len(message)>500{message=message[:500]}
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		status:="pending";if job.Attempts>=5{status="failed"}
		delay:=job.Attempts*30;if delay>900{delay=900};if delay<30{delay=30}
		res,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status=$3,last_error=$4,available_at=NOW()+($5 * INTERVAL '1 second'),lease_until=NULL,updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='processing'`,job.TenantID,job.ID,status,message,delay)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("reply job lease not found")};return nil
	})
}

func (s *Service) GetReply(ctx context.Context,tenantID,jobID string)(ReplyJob,error){
	if !validUUID(jobID){return ReplyJob{},errors.New("reply job not found")}
	var job ReplyJob
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		return tx.QueryRowContext(ctx,`SELECT id::text,tenant_id::text,telegram_account_id::text,chat_id,reply_to_message_id,message_text,attempts,status,COALESCE(telegram_message_id,0),last_error,created_at FROM message_reply_jobs WHERE tenant_id=$1 AND id=$2`,tenantID,jobID).Scan(&job.ID,&job.TenantID,&job.AccountID,&job.ChatID,&job.ReplyToMessageID,&job.Text,&job.Attempts,&job.Status,&job.TelegramMessageID,&job.LastError,&job.CreatedAt)
	})
	return job,err
}

func (s *Service) ReleaseReply(ctx context.Context,job ReplyJob,delay time.Duration) error {
	if delay<5*time.Second {delay=5*time.Second};if delay>5*time.Minute{delay=5*time.Minute}
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		res,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status='pending',attempts=GREATEST(attempts-1,0),available_at=NOW()+($3 * INTERVAL '1 second'),lease_until=NULL,updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='processing'`,job.TenantID,job.ID,int(delay.Seconds()))
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("reply job lease not found")};return nil
	})
}
