package messages

import (
	"context"
	"database/sql"
	"errors"
	"encoding/json"
	"strings"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type Service struct {
 db *sql.DB
 redis *redis.Client
}
func New(db *sql.DB, cache ...*redis.Client) *Service {
 s := &Service{db:db}
 if len(cache)>0 { s.redis=cache[0] }
 return s
}

// MessageCreatedEvent is published after a new message has committed to PostgreSQL.
type MessageCreatedEvent struct {
 Type string `json:"type"`
 AccountID string `json:"account_id"`
 ChatID int64 `json:"chat_id"`
 MessageID int64 `json:"message_id"`
 CreatedAt time.Time `json:"created_at"`
}

func (s *Service) publishCreated(ctx context.Context, tenantID string, in Incoming) {
 if s.redis==nil { return }
 payload,err:=json.Marshal(MessageCreatedEvent{
  Type:"message.created", AccountID:in.AccountID, ChatID:in.ChatID,
  MessageID:in.MessageID, CreatedAt:time.Now().UTC(),
 })
 if err!=nil { return }
 publishCtx,cancel:=context.WithTimeout(ctx,2*time.Second)
 defer cancel()
 _=s.redis.Publish(publishCtx,"tgqf:tenant:"+tenantID+":messages",string(payload)).Err()
}

// SubscribeMessages subscribes to one tenant's realtime message stream.
func (s *Service) SubscribeMessages(ctx context.Context, tenantID string) *redis.PubSub {
 if s.redis==nil { return nil }
 return s.redis.Subscribe(ctx,"tgqf:tenant:"+tenantID+":messages")
}

type Incoming struct {
	AccountID string
	ChatID int64
	MessageID int64
	ChatType string
	ChatTitle string
	ChatUsername string
	ChatMemberCount int
	ChatVerified bool
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

func ShouldCatalogChat(chatType string) bool { return chatType == "private" || chatType == "group" || chatType == "channel" }

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
	if !ShouldCatalogChat(in.ChatType) { return false,nil }
	if len(in.Text)>16384 || len(in.MediaType)>40 { return false,errors.New("message payload too large") }
	if in.MessageDate.IsZero() { in.MessageDate=time.Now().UTC() }
	collected:=false
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		var exists bool
		if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2)",tenantID,in.AccountID).Scan(&exists);err!=nil{return err}
		if !exists{return errors.New("account not found")}
		// Keep the chat catalogue independent from message-content collection:
		// observing a group must not require a reply to our own message or pass
		// the tenant's text/media filters. Only metadata is stored here.
		_,err:=tx.ExecContext(ctx,`INSERT INTO tenant_message_chats(tenant_id,telegram_account_id,chat_id,chat_title,chat_type,username,member_count,is_verified,last_seen_at,updated_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,NOW(),NOW())
			ON CONFLICT(tenant_id,telegram_account_id,chat_id) DO UPDATE SET
			chat_title=COALESCE(NULLIF(EXCLUDED.chat_title,''),tenant_message_chats.chat_title),
			chat_type=EXCLUDED.chat_type,
			username=COALESCE(NULLIF(EXCLUDED.username,''),tenant_message_chats.username),
			member_count=GREATEST(EXCLUDED.member_count,tenant_message_chats.member_count),
			is_verified=tenant_message_chats.is_verified OR EXCLUDED.is_verified,
			last_seen_at=NOW(),updated_at=NOW()`,tenantID,in.AccountID,in.ChatID,strings.TrimSpace(in.ChatTitle),in.ChatType,strings.TrimPrefix(strings.TrimSpace(in.ChatUsername),"@"),in.ChatMemberCount,in.ChatVerified)
		if err!=nil{return err}
		if in.ChatType=="group" || in.ChatType=="channel" {
			var groupID int64
			err=tx.QueryRowContext(ctx,`INSERT INTO public_groups(telegram_chat_id,chat_type,title,username,member_count,is_verified,first_seen_at,last_seen_at,updated_at)
				VALUES($1,$2,$3,$4,GREATEST($5,0),$6,NOW(),NOW(),NOW())
				ON CONFLICT(chat_type,telegram_chat_id) DO UPDATE SET
				 title=COALESCE(NULLIF(EXCLUDED.title,''),public_groups.title),
				 username=COALESCE(NULLIF(EXCLUDED.username,''),public_groups.username),
				 member_count=GREATEST(public_groups.member_count,EXCLUDED.member_count),
				 is_verified=public_groups.is_verified OR EXCLUDED.is_verified,
				 last_seen_at=NOW(),updated_at=NOW()
				RETURNING id`,in.ChatID,in.ChatType,strings.TrimSpace(in.ChatTitle),strings.TrimPrefix(strings.TrimSpace(in.ChatUsername),"@"),in.ChatMemberCount,in.ChatVerified).Scan(&groupID)
			if err!=nil{return err}
			_,err=tx.ExecContext(ctx,`INSERT INTO public_group_sources(group_id,tenant_id,telegram_account_id,source_chat_id,first_seen_at,last_seen_at,discovery_count)
				VALUES($1,$2,$3,$4,NOW(),NOW(),1)
				ON CONFLICT(group_id,tenant_id,telegram_account_id) DO UPDATE SET last_seen_at=NOW(),discovery_count=public_group_sources.discovery_count+1`,
				groupID,tenantID,in.AccountID,in.ChatID)
			if err!=nil{return err}
			_,err=tx.ExecContext(ctx,`INSERT INTO tenant_group_contributions(tenant_id,group_id,first_contributed_at,last_contributed_at,contribution_count)
				VALUES($1,$2,NOW(),NOW(),1)
				ON CONFLICT(tenant_id,group_id) DO UPDATE SET last_contributed_at=NOW(),contribution_count=tenant_group_contributions.contribution_count+1`,
				tenantID,groupID)
			if err!=nil{return err}
		}
		if !ShouldCollect(in.ChatType,in.IsOutgoing,in.ReplyToOwnMessage) { return nil }
		settings,err:=loadSettings(ctx,tx,tenantID);if err!=nil{return err}
		if !settings.Allows(in){return nil}
		result,insertErr:=tx.ExecContext(ctx,"INSERT INTO unified_messages(tenant_id,telegram_account_id,chat_id,message_id,chat_type,chat_title,sender_id,reply_to_message_id,reply_to_own_message,is_outgoing,message_text,media_type,message_date) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,0),$9,FALSE,$10,$11,$12) ON CONFLICT(telegram_account_id,chat_id,message_id) DO NOTHING",
			tenantID,in.AccountID,in.ChatID,in.MessageID,in.ChatType,strings.TrimSpace(in.ChatTitle),in.SenderID,in.ReplyToMessageID,in.ReplyToOwnMessage,strings.TrimSpace(in.Text),in.MediaType,in.MessageDate)
		if insertErr!=nil { return insertErr }
		inserted,_:=result.RowsAffected()
		collected=inserted>0
		return nil
	})
	if err!=nil{return false,err}
	if collected { s.publishCreated(ctx,tenantID,in) }
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
	ChatUsername string `json:"chat_username"`
	MemberCount int `json:"member_count"`
	IsVerified bool `json:"is_verified"`
}

func (s *Service) ListChats(ctx context.Context,tenantID string,limit,offset int)([]ChatSummary,int,error){
	if limit<1||limit>100{limit=50};if offset<0{offset=0}
	out:=[]ChatSummary{};total:=0
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		if err:=tx.QueryRowContext(ctx,"SELECT COUNT(DISTINCT (telegram_account_id,chat_id)) FROM unified_messages WHERE tenant_id=$1",tenantID).Scan(&total);err!=nil{return err}
		query:=`SELECT g.telegram_account_id::text,COALESCE(NULLIF(a.display_name,''),a.phone,''),g.chat_id,
			COALESCE(NULLIF(cm.chat_title,''),g.chat_title),g.chat_type,g.unprocessed_count,COALESCE(latest.message_text,''),g.last_message_date,
			COALESCE(cm.username,''),COALESCE(cm.member_count,0),COALESCE(cm.is_verified,FALSE)
			FROM (SELECT telegram_account_id,chat_id,MAX(chat_title) AS chat_title,MAX(chat_type) AS chat_type,COUNT(*) FILTER (WHERE processed_at IS NULL) AS unprocessed_count,MAX(message_date) AS last_message_date FROM unified_messages WHERE tenant_id=$1 GROUP BY telegram_account_id,chat_id) g
			JOIN telegram_accounts a ON a.tenant_id=$1 AND a.id=g.telegram_account_id
			LEFT JOIN tenant_message_chats cm ON cm.tenant_id=$1 AND cm.telegram_account_id=g.telegram_account_id AND cm.chat_id=g.chat_id
			LEFT JOIN LATERAL (SELECT message_text FROM unified_messages m2 WHERE m2.tenant_id=$1 AND m2.telegram_account_id=g.telegram_account_id AND m2.chat_id=g.chat_id ORDER BY m2.message_date DESC,m2.id DESC LIMIT 1) latest ON TRUE
			ORDER BY g.last_message_date DESC LIMIT $2 OFFSET $3`
		rows,err:=tx.QueryContext(ctx,query,tenantID,limit,offset);if err!=nil{return err};defer rows.Close()
		for rows.Next(){var item ChatSummary;if err:=rows.Scan(&item.AccountID,&item.AccountName,&item.ChatID,&item.ChatTitle,&item.ChatType,&item.UnprocessedCount,&item.LastMessage,&item.LastMessageDate,&item.ChatUsername,&item.MemberCount,&item.IsVerified);err!=nil{return err};out=append(out,item)}
		return rows.Err()
	})
	return out,total,err
}


var ErrInvalidGroupLibraryQuery = errors.New("invalid group library query")

type GroupLibraryItem struct {
	ChatID int64 `json:"chat_id"`
	Title string `json:"title"`
	ChatType string `json:"chat_type"`
	Username string `json:"username"`
	MemberCount int `json:"member_count"`
	IsVerified bool `json:"is_verified"`
	SourceCount int `json:"source_count"`
	ContributorCount int `json:"contributor_count"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

func normalizeGroupLibraryFilters(chatType, query string) (string, string, error) {
	chatType = strings.ToLower(strings.TrimSpace(chatType))
	if chatType != "" && chatType != "group" && chatType != "channel" {
		return "", "", ErrInvalidGroupLibraryQuery
	}
	query = strings.TrimSpace(query)
	if len(query) > 100 { return "", "", ErrInvalidGroupLibraryQuery }
	return chatType, query, nil
}

// ListGroupLibrary returns the shared system catalogue. Tenant identity is
// retained for authorization context, but the catalogue itself is global.
func (s *Service) ListGroupLibrary(ctx context.Context, tenantID, chatType, query string, limit, offset int) ([]GroupLibraryItem, int, error) {
	if limit < 1 || limit > 100 { limit = 50 }
	if offset < 0 { offset = 0 }
	chatType, query, err := normalizeGroupLibraryFilters(chatType, query)
	if err != nil { return nil, 0, err }
	items := []GroupLibraryItem{}
	total := 0
	err = withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		filter := "1=1"
		args := []any{}
		if chatType != "" { args=append(args,chatType);filter+=" AND g.chat_type=$"+strconv.Itoa(len(args)) }
		if query != "" { args=append(args,"%"+query+"%");n:=strconv.Itoa(len(args));filter+=" AND (g.title ILIKE $"+n+" OR g.username ILIKE $"+n+")" }
		if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM public_groups g WHERE "+filter,args...).Scan(&total);err!=nil{return err}
		args=append(args,limit,offset);limitArg:=strconv.Itoa(len(args)-1);offsetArg:=strconv.Itoa(len(args))
		rows,err:=tx.QueryContext(ctx,`SELECT g.telegram_chat_id,g.title,g.chat_type,g.username,g.member_count,g.is_verified,
			(SELECT COUNT(*) FROM public_group_sources s WHERE s.group_id=g.id),
			(SELECT COUNT(*) FROM tenant_group_contributions c WHERE c.group_id=g.id),
			g.last_seen_at
			FROM public_groups g WHERE `+filter+` ORDER BY g.last_seen_at DESC,g.title ASC,g.id ASC LIMIT $`+limitArg+` OFFSET $`+offsetArg,args...)
		if err!=nil{return err};defer rows.Close()
		for rows.Next(){var item GroupLibraryItem;if err:=rows.Scan(&item.ChatID,&item.Title,&item.ChatType,&item.Username,&item.MemberCount,&item.IsVerified,&item.SourceCount,&item.ContributorCount,&item.LastSeenAt);err!=nil{return err};items=append(items,item)}
		return rows.Err()
	})
	return items,total,err
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
	// Expired/disabled tenants are swept before any active tenant can claim work.
	inactiveRows,err:=s.db.QueryContext(ctx,`SELECT id::text FROM tenants WHERE status<>'active' OR expires_at IS NULL OR expires_at<=NOW()`)
	if err!=nil{return nil,err}
	var inactiveTenants []string
	for inactiveRows.Next(){var id string;if err:=inactiveRows.Scan(&id);err!=nil{inactiveRows.Close();return nil,err};inactiveTenants=append(inactiveTenants,id)}
	if err:=inactiveRows.Err();err!=nil{inactiveRows.Close();return nil,err};inactiveRows.Close()
	for _,tenantID:=range inactiveTenants {
		if err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
			_,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status='paused',lease_until=NULL,last_error='tenant inactive; manual review required',updated_at=NOW() WHERE tenant_id=$1 AND status IN ('pending','processing')`,tenantID)
			return err
		});err!=nil{return nil,err}
	}
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

func (s *Service) TenantCanOperate(ctx context.Context, tenantID string) bool {
	var allowed bool
	err:=s.db.QueryRowContext(ctx,`SELECT status='active' AND expires_at>NOW() FROM tenants WHERE id=$1`,tenantID).Scan(&allowed)
	return err==nil && allowed
}


func (s *Service) ResumeReply(ctx context.Context, tenantID, jobID string) error {
	if !validUUID(jobID) { return errors.New("reply job not found") }
	return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		res,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs j
			SET status='pending',available_at=NOW(),lease_until=NULL,last_error='',updated_at=NOW()
			FROM tenants t,telegram_accounts a
			WHERE j.tenant_id=$1 AND j.id=$2 AND j.status='paused'
			AND t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW()
			AND a.tenant_id=j.tenant_id AND a.id=j.telegram_account_id
			AND a.status='authorized' AND a.session_ciphertext IS NOT NULL`,tenantID,jobID)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("paused reply cannot be resumed; verify tenant, account, and job status")}
		return nil
	})
}

func (s *Service) PauseReply(ctx context.Context, job ReplyJob, reason string) error {
	reason=strings.TrimSpace(reason);if reason==""{reason="tenant inactive; manual review required"};if len(reason)>500{reason=reason[:500]}
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		_,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status='paused',lease_until=NULL,last_error=$3,updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='processing'`,job.TenantID,job.ID,reason)
		return err
	})
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


// ListKnownChatIDs returns a bounded set of chats already discovered by this
// tenant/account. It deliberately does not enumerate arbitrary Telegram dialogs.
func (s *Service) ListKnownChatIDs(ctx context.Context, tenantID, accountID string, limit int) ([]int64, error) {
	if !validUUID(accountID) { return nil, errors.New("invalid account id") }
	if limit < 1 || limit > 50 { limit = 20 }
	out := []int64{}
	err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT chat_id FROM tenant_message_chats
			WHERE tenant_id=$1 AND telegram_account_id=$2
			ORDER BY last_seen_at DESC LIMIT $3`, tenantID, accountID, limit)
		if err != nil { return err }
		defer rows.Close()
		for rows.Next() { var id int64; if err := rows.Scan(&id); err != nil { return err }; out = append(out,id) }
		return rows.Err()
	})
	return out, err
}
