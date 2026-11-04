package messages

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"fmt"
	"strings"
	"time"
	"unicode"

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
 key := "tgqf:tenant:"+tenantID+":message-events"
 // Redis Streams make reconnects resumable; keep a bounded recent history per tenant.
 _ = s.redis.XAdd(publishCtx,&redis.XAddArgs{
  Stream:key, MaxLen:10000, Approx:true,
  Values:map[string]any{"payload":string(payload)},
 }).Err()
}

// MessageStreamEvent is one durable event in the tenant-scoped Redis stream.
type MessageStreamEvent struct {
 ID string `json:"id"`
 Payload string `json:"payload"`
}

// ReadMessageEvents reads events after the supplied Redis stream cursor.
// Use "$" for a new connection and the last received ID when reconnecting.
func (s *Service) ReadMessageEvents(ctx context.Context, tenantID, cursor string, block time.Duration, count int64) ([]MessageStreamEvent, error) {
 if s.redis==nil { return nil, errors.New("realtime message stream is unavailable") }
 if strings.TrimSpace(tenantID)=="" { return nil, errors.New("tenant is required") }
 if cursor=="" { cursor="$" }
 if block<=0 || block>30*time.Second { block=15*time.Second }
 if count<1 || count>500 { count=100 }
 streams,err:=s.redis.XRead(ctx,&redis.XReadArgs{
  Streams:[]string{"tgqf:tenant:"+tenantID+":message-events",cursor},
  Count:count,Block:block,
 }).Result()
 if err!=nil {
  if errors.Is(err,redis.Nil) { return nil,nil }
  return nil,err
 }
 events:=make([]MessageStreamEvent,0)
 for _,stream:=range streams {
  for _,message:=range stream.Messages {
   payload,ok:=message.Values["payload"].(string)
   if !ok { continue }
   events=append(events,MessageStreamEvent{ID:message.ID,Payload:payload})
  }
 }
 return events,nil
}

func nullJSON(raw []byte) any { if len(raw)==0 { return nil }; return string(raw) }

type Incoming struct {
	AccountID string
	ChatID int64
	MessageID int64
	ChatType string
	ChatTitle string
	ChatUsername string
	SenderUsername string
	SenderName string
	ChatMemberCount int
	ChatVerified bool
	DefaultPermissionsJSON []byte
	AccountRightsJSON []byte
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

type ListOptions struct { Limit int; Offset int; IncludeProcessed bool; AccountID string; ChatID int64; ChatType string; MediaType string; Query string }

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
		// A Redis delivery may be replayed after a worker crash or an ACK failure.
		// Claim the Telegram update inside this transaction so every downstream
		// side effect (catalogue counters, link sources, keyword matches, messages)
		// is applied at most once.
		dedup,err:=tx.ExecContext(ctx,`INSERT INTO telegram_incoming_update_dedup(tenant_id,telegram_account_id,chat_id,message_id)
			VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`,tenantID,in.AccountID,in.ChatID,in.MessageID)
		if err!=nil{return err}
		claimed,err:=dedup.RowsAffected()
		if err!=nil{return err}
		if claimed==0{return nil}
		// Keep the chat catalogue independent from message-content collection:
		// observing a group must not require a reply to our own message or pass
		// the tenant's text/media filters. Only metadata is stored here.
		_,err=tx.ExecContext(ctx,`INSERT INTO tenant_message_chats(tenant_id,telegram_account_id,chat_id,chat_title,chat_type,username,member_count,is_verified,last_seen_at,updated_at)
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
			err=tx.QueryRowContext(ctx,`INSERT INTO public_groups(telegram_chat_id,chat_type,title,username,member_count,is_verified,default_permissions,permissions_observed_at,first_seen_at,last_seen_at,updated_at)
				VALUES($1,$2,$3,$4,GREATEST($5,0),$6,$7,CASE WHEN $7::jsonb IS NULL THEN NULL ELSE NOW() END,NOW(),NOW(),NOW())
				ON CONFLICT(chat_type,telegram_chat_id) DO UPDATE SET
				 title=COALESCE(NULLIF(EXCLUDED.title,''),public_groups.title),
				 username=COALESCE(NULLIF(EXCLUDED.username,''),public_groups.username),
				 member_count=GREATEST(public_groups.member_count,EXCLUDED.member_count),
				 is_verified=public_groups.is_verified OR EXCLUDED.is_verified,
				 default_permissions=COALESCE(EXCLUDED.default_permissions,public_groups.default_permissions),
				 permissions_observed_at=CASE WHEN EXCLUDED.default_permissions IS NULL THEN public_groups.permissions_observed_at ELSE NOW() END,
				 last_seen_at=NOW(),updated_at=NOW()
				RETURNING id`,in.ChatID,in.ChatType,strings.TrimSpace(in.ChatTitle),strings.TrimPrefix(strings.TrimSpace(in.ChatUsername),"@"),in.ChatMemberCount,in.ChatVerified,nullJSON(in.DefaultPermissionsJSON)).Scan(&groupID)
			if err!=nil{return err}
			if username:=strings.TrimPrefix(strings.TrimSpace(in.ChatUsername),"@");username!="" {
				if _,err=tx.ExecContext(ctx,`UPDATE public_group_links SET group_id=$1,last_seen_at=NOW() WHERE link_type='public' AND group_id IS NULL AND lower(public_url)=lower($2)`,groupID,"https://t.me/"+username);err!=nil{return err}
			}
			_,err=tx.ExecContext(ctx,`INSERT INTO public_group_sources(group_id,tenant_id,telegram_account_id,source_chat_id,account_rights_snapshot,account_rights_observed_at,first_seen_at,last_seen_at,discovery_count)
				VALUES($1,$2,$3,$4,$5,CASE WHEN $5::jsonb IS NULL THEN NULL ELSE NOW() END,NOW(),NOW(),1)
				ON CONFLICT(group_id,tenant_id,telegram_account_id) DO UPDATE SET last_seen_at=NOW(),discovery_count=public_group_sources.discovery_count+1,
				 account_rights_snapshot=COALESCE(EXCLUDED.account_rights_snapshot,public_group_sources.account_rights_snapshot),
				 account_rights_observed_at=CASE WHEN EXCLUDED.account_rights_snapshot IS NULL THEN public_group_sources.account_rights_observed_at ELSE NOW() END`,
				groupID,tenantID,in.AccountID,in.ChatID,nullJSON(in.AccountRightsJSON))
			if err!=nil{return err}
			_,err=tx.ExecContext(ctx,`INSERT INTO tenant_group_contributions(tenant_id,group_id,first_contributed_at,last_contributed_at,contribution_count)
				VALUES($1,$2,NOW(),NOW(),1)
				ON CONFLICT(tenant_id,group_id) DO UPDATE SET last_contributed_at=NOW(),contribution_count=tenant_group_contributions.contribution_count+1`,
				tenantID,groupID)
			if err!=nil{return err}
		}
		// Link discovery runs only on this live incoming update. It never queries history.
		if !in.IsOutgoing && strings.TrimSpace(in.Text)!="" {
			linkSettings,err:=loadLinkCollectionSettings(ctx,tx,tenantID);if err!=nil{return err}
			if linkSettings.Allows(in.ChatType) {
				if err:=storeDiscoveredLinks(ctx,tx,tenantID,in);err!=nil{return err}
			}
		}
		// Keyword rules inspect only this live incoming update; they never trigger outbound actions.
		if err:=evaluateAndRecordKeywordMatches(ctx,tx,tenantID,in);err!=nil{return err}
		if !ShouldCollect(in.ChatType,in.IsOutgoing,in.ReplyToOwnMessage) { return nil }
		settings,err:=loadSettings(ctx,tx,tenantID);if err!=nil{return err}
		if !settings.Allows(in){return nil}
		result,insertErr:=tx.ExecContext(ctx,"INSERT INTO unified_messages(tenant_id,telegram_account_id,chat_id,message_id,chat_type,chat_title,sender_id,reply_to_message_id,reply_to_own_message,is_outgoing,message_text,media_type,message_date) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,0),$9,FALSE,$10,$11,$12) ON CONFLICT(telegram_account_id,chat_id,message_id) DO NOTHING",
			tenantID,in.AccountID,in.ChatID,in.MessageID,in.ChatType,strings.TrimSpace(in.ChatTitle),in.SenderID,in.ReplyToMessageID,in.ReplyToOwnMessage,strings.TrimSpace(in.Text),in.MediaType,in.MessageDate)
		if insertErr!=nil { return insertErr }
		inserted,_:=result.RowsAffected()
		collected=inserted>0
		if collected {
			payload,marshalErr:=json.Marshal(MessageCreatedEvent{
				Type:"message.created",AccountID:in.AccountID,ChatID:in.ChatID,
				MessageID:in.MessageID,CreatedAt:time.Now().UTC(),
			})
			if marshalErr!=nil{return marshalErr}
			if _,outboxErr:=tx.ExecContext(ctx,`INSERT INTO message_event_outbox(tenant_id,event_type,payload)
				VALUES($1,'message.created',$2::jsonb)`,tenantID,string(payload));outboxErr!=nil{return outboxErr}
		}
		return nil
	})
	if err!=nil{return false,err}
	return collected,nil
}


func storeDiscoveredLinks(ctx context.Context,tx *sql.Tx,tenantID string,in Incoming)error{
	for _,link:=range ExtractTelegramLinks(in.Text){
		var id int64
		if link.Type=="public" {
			if err:=tx.QueryRowContext(ctx,`INSERT INTO public_group_links(group_id,link_type,public_url,status)
				VALUES((SELECT id FROM public_groups WHERE lower(username)=lower($1) ORDER BY id LIMIT 1),'public',$2,'pending')
				ON CONFLICT(link_type,public_url) DO UPDATE SET
				 last_seen_at=NOW(),
				 status=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.status ELSE 'pending' END,
				 last_checked_at=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.last_checked_at ELSE NULL END,
				 last_check_error=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.last_check_error ELSE NULL END,
				 check_attempts=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.check_attempts ELSE 0 END,
				 next_check_at=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.next_check_at ELSE NULL END,
				 check_lease_until=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.check_lease_until ELSE NULL END,
				 check_lease_tenant_id=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.check_lease_tenant_id ELSE NULL END,
				 check_lease_account_id=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.check_lease_account_id ELSE NULL END
				RETURNING id`,strings.TrimPrefix(strings.TrimPrefix(link.PublicURL,"https://t.me/"),"@"),link.PublicURL).Scan(&id);err!=nil{return err}
		}else if link.Type=="invite" {
			if len(link.InviteHash)==0||len(link.InviteCiphertext)==0{continue}
			if err:=tx.QueryRowContext(ctx,`INSERT INTO public_group_links(link_type,invite_hash,invite_ciphertext,status)
				VALUES('invite',$1,$2,'pending')
				ON CONFLICT(invite_hash) DO UPDATE SET
				 last_seen_at=NOW(),
				 status=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.status ELSE 'pending' END,
				 last_checked_at=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.last_checked_at ELSE NULL END,
				 last_check_error=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.last_check_error ELSE NULL END,
				 check_attempts=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.check_attempts ELSE 0 END,
				 next_check_at=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.next_check_at ELSE NULL END,
				 check_lease_until=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.check_lease_until ELSE NULL END,
				 check_lease_tenant_id=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.check_lease_tenant_id ELSE NULL END,
				 check_lease_account_id=CASE WHEN public_group_links.check_lease_until>NOW() THEN public_group_links.check_lease_account_id ELSE NULL END
				RETURNING id`,link.InviteHash,link.InviteCiphertext).Scan(&id);err!=nil{return err}
		}else{continue}
		_,err:=tx.ExecContext(ctx,`INSERT INTO public_group_link_sources(link_id,tenant_id,telegram_account_id,source_chat_id,source_message_id)
			VALUES($1,$2,$3,$4,$5)
			ON CONFLICT(link_id,tenant_id,telegram_account_id,source_chat_id,source_message_id)
			DO UPDATE SET last_seen_at=NOW(),discovery_count=public_group_link_sources.discovery_count+1`,
			id,tenantID,in.AccountID,in.ChatID,in.MessageID)
		if err!=nil{return err}
	}
	return nil
}

func (s *Service) List(ctx context.Context,tenantID string,opts ListOptions)([]Message,int,error){
	if opts.Limit<1||opts.Limit>100{opts.Limit=50};if opts.Offset<0{opts.Offset=0}
	opts.AccountID=strings.TrimSpace(opts.AccountID)
	opts.ChatType=strings.ToLower(strings.TrimSpace(opts.ChatType))
	opts.MediaType=strings.ToLower(strings.TrimSpace(opts.MediaType))
	opts.Query=strings.TrimSpace(opts.Query)
	if opts.AccountID!=""&&!validUUID(opts.AccountID){return nil,0,errors.New("invalid account filter")}
	if opts.ChatType!=""&&opts.ChatType!="private"&&opts.ChatType!="group"&&opts.ChatType!="channel"{return nil,0,errors.New("invalid chat type filter")}
	if len(opts.MediaType)>32||len(opts.Query)>120{return nil,0,errors.New("invalid message filter")}
	out:=[]Message{};total:=0
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		filter:="m.tenant_id=$1"
		args:=[]any{tenantID}
		if !opts.IncludeProcessed{filter+=" AND m.processed_at IS NULL"}
		if opts.AccountID!=""{args=append(args,opts.AccountID);filter+=fmt.Sprintf(" AND m.telegram_account_id=$%d",len(args))}
		if opts.ChatID!=0{args=append(args,opts.ChatID);filter+=fmt.Sprintf(" AND m.chat_id=$%d",len(args))}
		if opts.ChatType!=""{args=append(args,opts.ChatType);filter+=fmt.Sprintf(" AND m.chat_type=$%d",len(args))}
		if opts.MediaType!=""{args=append(args,opts.MediaType);filter+=fmt.Sprintf(" AND m.media_type=$%d",len(args))}
		if opts.Query!=""{args=append(args,"%"+opts.Query+"%");filter+=fmt.Sprintf(" AND (m.message_text ILIKE $%d OR m.chat_title ILIKE $%d)",len(args),len(args))}
		if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM unified_messages m WHERE "+filter,args...).Scan(&total);err!=nil{return err}
		query:="SELECT m.id::text,m.telegram_account_id::text,COALESCE(NULLIF(a.display_name,''),a.phone,''),m.chat_id,m.message_id,m.chat_type,m.chat_title,m.sender_id,COALESCE(m.reply_to_message_id,0),m.message_text,m.media_type,m.message_date,m.processed_at FROM unified_messages m JOIN telegram_accounts a ON a.tenant_id=m.tenant_id AND a.id=m.telegram_account_id WHERE "+filter+" ORDER BY m.message_date DESC,m.id DESC"
		listArgs:=append(append([]any{},args...),opts.Limit,opts.Offset)
		query+=fmt.Sprintf(" LIMIT $%d OFFSET $%d",len(listArgs)-1,len(listArgs))
		rows,err:=tx.QueryContext(ctx,query,listArgs...);if err!=nil{return err};defer rows.Close()
		for rows.Next(){var m Message;if err:=rows.Scan(&m.ID,&m.AccountID,&m.AccountName,&m.ChatID,&m.MessageID,&m.ChatType,&m.ChatTitle,&m.SenderID,&m.ReplyToMessageID,&m.Text,&m.MediaType,&m.MessageDate,&m.ProcessedAt);err!=nil{return err};out=append(out,m)}
		return rows.Err()
	})
	return out,total,err
}


// MediaIdentity is resolved only from a message row visible to the current tenant.
type MediaIdentity struct { AccountID string; ChatID int64; MessageID int64; MediaType string }
func (s *Service) GetMediaIdentity(ctx context.Context,tenantID,messageID string)(MediaIdentity,error){
 if !validUUID(messageID){return MediaIdentity{},errors.New("message not found")}
 var out MediaIdentity
 err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
  return tx.QueryRowContext(ctx,`SELECT telegram_account_id::text,chat_id,message_id,media_type FROM unified_messages WHERE tenant_id=$1 AND id=$2`,tenantID,messageID).Scan(&out.AccountID,&out.ChatID,&out.MessageID,&out.MediaType)
 })
 if errors.Is(err,sql.ErrNoRows){return MediaIdentity{},errors.New("message not found")}
 return out,err
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
	DefaultPermissions json.RawMessage `json:"default_permissions,omitempty"`
	PermissionsObservedAt *time.Time `json:"permissions_observed_at,omitempty"`
	ID int64 `json:"id"`
	ChatID int64 `json:"chat_id"`
	Title string `json:"title"`
	ChatType string `json:"chat_type"`
	Username string `json:"username"`
	MemberCount int `json:"member_count"`
	IsVerified bool `json:"is_verified"`
	SourceCount int `json:"source_count"`
	ContributorCount int `json:"contributor_count"`
	LastSeenAt time.Time `json:"last_seen_at"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastCheckError string `json:"last_check_error,omitempty"`
	CheckAttempts int `json:"check_attempts"`
	NextCheckAt *time.Time `json:"next_check_at,omitempty"`
}

type GroupLibraryFilter struct {
	ChatType string
	Query string
	Verified *bool
	MinMembers int
	MaxMembers int
	MinSources int
	MinContributors int
	Sort string
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

func validateGroupLibraryFilter(filter GroupLibraryFilter) error {
	if filter.MinMembers < 0 || filter.MaxMembers < 0 || filter.MinSources < 0 || filter.MinContributors < 0 || (filter.MaxMembers > 0 && filter.MinMembers > filter.MaxMembers) { return ErrInvalidGroupLibraryQuery }
	switch filter.Sort { case "", "recent", "title", "members", "sources", "contributors": return nil; default: return ErrInvalidGroupLibraryQuery }
}

// ListGroupLibrary returns the shared system catalogue. Tenant identity is
// retained for authorization context, but the catalogue itself is global.
func (s *Service) ListGroupLibrary(ctx context.Context, tenantID, chatType, query string, limit, offset int) ([]GroupLibraryItem, int, error) {
	return s.ListGroupLibraryFiltered(ctx,tenantID,GroupLibraryFilter{ChatType:chatType,Query:query},limit,offset)
}

// ListGroupLibraryFiltered exposes stable filtering and sorting over the shared catalogue.
func (s *Service) ListGroupLibraryFiltered(ctx context.Context,tenantID string,filter GroupLibraryFilter,limit,offset int)([]GroupLibraryItem,int,error) {
	if limit<1||limit>100 {limit=50};if offset<0 {offset=0}
	chatType,query,err:=normalizeGroupLibraryFilters(filter.ChatType,filter.Query);if err!=nil{return nil,0,err}
	if err:=validateGroupLibraryFilter(filter);err!=nil{return nil,0,err}
	sortExpr:="g.last_seen_at DESC,g.title ASC,g.id ASC"
	switch filter.Sort {case "", "recent": case "title":sortExpr="lower(g.title) ASC,g.id ASC";case "members":sortExpr="g.member_count DESC,g.title ASC,g.id ASC";case "sources":sortExpr="(SELECT COUNT(*) FROM public_group_sources s WHERE s.group_id=g.id AND s.tenant_id=scope.tenant_id) DESC,g.last_seen_at DESC,g.id ASC";case "contributors":sortExpr="(SELECT COUNT(*) FROM tenant_group_contributions c WHERE c.group_id=g.id AND c.tenant_id=scope.tenant_id) DESC,g.last_seen_at DESC,g.id ASC"}
	items:=[]GroupLibraryItem{};total:=0
	err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		clauses:=[]string{"1=1"};args:=[]any{tenantID}
		add:=func(value any,clause string){args=append(args,value);clauses=append(clauses,fmt.Sprintf(clause,len(args)))}
		if chatType!=""{add(chatType,"g.chat_type=$%d")}
		if query!=""{args=append(args,query);n:=len(args);clauses=append(clauses,fmt.Sprintf("(strpos(lower(g.title),lower($%d))>0 OR strpos(lower(g.username),lower($%d))>0)",n,n));}
		if filter.Verified!=nil{add(*filter.Verified,"g.is_verified=$%d")}
		if filter.MinMembers>0{add(filter.MinMembers,"g.member_count >= $%d")}
		if filter.MaxMembers>0{add(filter.MaxMembers,"g.member_count <= $%d")}
		if filter.MinSources>0{add(filter.MinSources,"(SELECT COUNT(*) FROM public_group_sources s WHERE s.group_id=g.id AND s.tenant_id=scope.tenant_id) >= $%d")}
		if filter.MinContributors>0{add(filter.MinContributors,"(SELECT COUNT(*) FROM tenant_group_contributions c WHERE c.group_id=g.id AND c.tenant_id=scope.tenant_id) >= $%d")}
		where:=strings.Join(clauses," AND ")
		if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM public_groups g CROSS JOIN (SELECT $1::uuid AS tenant_id) scope WHERE "+where,args...).Scan(&total);err!=nil{return err}
		args=append(args,limit,offset);limitArg:=len(args)-1;offsetArg:=len(args)
		querySQL:=fmt.Sprintf(`SELECT g.id,g.telegram_chat_id,g.title,g.chat_type,g.username,g.member_count,g.is_verified,g.default_permissions,g.permissions_observed_at,
			(SELECT COUNT(*) FROM public_group_sources s WHERE s.group_id=g.id AND s.tenant_id=scope.tenant_id),
			(SELECT COUNT(*) FROM tenant_group_contributions c WHERE c.group_id=g.id AND c.tenant_id=scope.tenant_id),g.last_seen_at
			FROM public_groups g CROSS JOIN (SELECT $1::uuid AS tenant_id) scope WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`,where,sortExpr,limitArg,offsetArg)
		rows,err:=tx.QueryContext(ctx,querySQL,args...);if err!=nil{return err};defer rows.Close()
		for rows.Next(){var item GroupLibraryItem;if err:=rows.Scan(&item.ID,&item.ChatID,&item.Title,&item.ChatType,&item.Username,&item.MemberCount,&item.IsVerified,&item.DefaultPermissions,&item.PermissionsObservedAt,&item.SourceCount,&item.ContributorCount,&item.LastSeenAt);err!=nil{return err};items=append(items,item)}
		return rows.Err()
	})
	return items,total,err
}

type GroupSource struct {
	AccountID string `json:"account_id"`
	AccountName string `json:"account_name"`
	ChatID int64 `json:"chat_id"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	DiscoveryCount int64 `json:"discovery_count"`
	AccountRights json.RawMessage `json:"account_rights,omitempty"`
	AccountRightsObservedAt *time.Time `json:"account_rights_observed_at,omitempty"`
}


var ErrGroupLibraryNotFound = errors.New("group not found")

// GetGroupLibraryItem returns one shared catalogue entry. Source details remain
// tenant-scoped and are fetched separately through ListGroupSources.
func (s *Service) GetGroupLibraryItem(ctx context.Context, tenantID string, groupID int64) (GroupLibraryItem, error) {
	if groupID <= 0 { return GroupLibraryItem{}, ErrInvalidGroupLibraryQuery }
	var item GroupLibraryItem
	err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `SELECT g.id,g.telegram_chat_id,g.title,g.chat_type,g.username,g.member_count,g.is_verified,
			g.default_permissions,g.permissions_observed_at,
			(SELECT COUNT(*) FROM public_group_sources src WHERE src.group_id=g.id AND src.tenant_id=$2),
			(SELECT COUNT(*) FROM tenant_group_contributions c WHERE c.group_id=g.id AND c.tenant_id=$2),g.last_seen_at
			FROM public_groups g WHERE g.id=$1`, groupID, tenantID).Scan(
			&item.ID,&item.ChatID,&item.Title,&item.ChatType,&item.Username,&item.MemberCount,&item.IsVerified,
			&item.DefaultPermissions,&item.PermissionsObservedAt,&item.SourceCount,&item.ContributorCount,&item.LastSeenAt)
		if errors.Is(err, sql.ErrNoRows) { return ErrGroupLibraryNotFound }
		return err
	})
	return item, err
}

func (s *Service) ListGroupSources(ctx context.Context, tenantID string, groupID int64) ([]GroupSource,error) {
	if groupID<=0 { return nil,ErrInvalidGroupLibraryQuery }
	items:=[]GroupSource{}
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		rows,err:=tx.QueryContext(ctx,`SELECT src.telegram_account_id::text,
			COALESCE(NULLIF(a.display_name,''),a.phone,''),src.source_chat_id,
			src.first_seen_at,src.last_seen_at,src.discovery_count,src.account_rights_snapshot,src.account_rights_observed_at
			FROM public_group_sources src
			JOIN telegram_accounts a ON a.tenant_id=src.tenant_id AND a.id=src.telegram_account_id
			WHERE src.tenant_id=$1 AND src.group_id=$2
			ORDER BY src.last_seen_at DESC,src.telegram_account_id`,tenantID,groupID)
		if err!=nil{return err};defer rows.Close()
		for rows.Next(){var item GroupSource;if err:=rows.Scan(&item.AccountID,&item.AccountName,&item.ChatID,&item.FirstSeenAt,&item.LastSeenAt,&item.DiscoveryCount,&item.AccountRights,&item.AccountRightsObservedAt);err!=nil{return err};items=append(items,item)}
		return rows.Err()
	})
	return items,err
}



type LinkCollectionSettings struct {
	Enabled bool `json:"enabled"`
	FromPrivateMessages bool `json:"from_private_messages"`
	FromGroups bool `json:"from_groups"`
	FromChannels bool `json:"from_channels"`
}

func DefaultLinkCollectionSettings() LinkCollectionSettings {
	return LinkCollectionSettings{Enabled:true,FromPrivateMessages:true,FromGroups:true,FromChannels:true}
}

func (s LinkCollectionSettings) Allows(chatType string) bool {
	if !s.Enabled { return false }
	switch chatType {
	case "private": return s.FromPrivateMessages
	case "group": return s.FromGroups
	case "channel": return s.FromChannels
	default: return false
	}
}

func loadLinkCollectionSettings(ctx context.Context,tx *sql.Tx,tenantID string)(LinkCollectionSettings,error){
	settings:=DefaultLinkCollectionSettings()
	err:=tx.QueryRowContext(ctx,`SELECT enabled,from_private_messages,from_groups,from_channels FROM tenant_link_collection_settings WHERE tenant_id=$1`,tenantID).
		Scan(&settings.Enabled,&settings.FromPrivateMessages,&settings.FromGroups,&settings.FromChannels)
	if errors.Is(err,sql.ErrNoRows){return settings,nil}
	return settings,err
}

func (s *Service) GetLinkCollectionSettings(ctx context.Context,tenantID string)(LinkCollectionSettings,error){
	settings:=DefaultLinkCollectionSettings()
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{var err error;settings,err=loadLinkCollectionSettings(ctx,tx,tenantID);return err})
	return settings,err
}

func (s *Service) UpdateLinkCollectionSettings(ctx context.Context,tenantID string,settings LinkCollectionSettings)(LinkCollectionSettings,error){
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		_,err:=tx.ExecContext(ctx,`INSERT INTO tenant_link_collection_settings(tenant_id,enabled,from_private_messages,from_groups,from_channels,updated_at)
			VALUES($1,$2,$3,$4,$5,NOW()) ON CONFLICT(tenant_id) DO UPDATE SET
			enabled=EXCLUDED.enabled,from_private_messages=EXCLUDED.from_private_messages,
			from_groups=EXCLUDED.from_groups,from_channels=EXCLUDED.from_channels,updated_at=NOW()`,
			tenantID,settings.Enabled,settings.FromPrivateMessages,settings.FromGroups,settings.FromChannels)
		return err
	})
	return settings,err
}

type GroupLinkItem struct {
	ID int64 `json:"id"`
	GroupID *int64 `json:"group_id,omitempty"`
	Type string `json:"type"`
	PublicURL string `json:"public_url,omitempty"`
	PrivateInvite bool `json:"private_invite"`
	Status string `json:"status"`
	SourceCount int `json:"source_count"`
	FirstSeenAt time.Time `json:"first_seen_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastCheckError string `json:"last_check_error,omitempty"`
	CheckAttempts int `json:"check_attempts"`
	NextCheckAt *time.Time `json:"next_check_at,omitempty"`
}

func (s *Service) ListGroupLinks(ctx context.Context, tenantID, linkType, status string, limit, offset int) ([]GroupLinkItem, int, error) {
	linkType = strings.ToLower(strings.TrimSpace(linkType))
	status = strings.ToLower(strings.TrimSpace(status))
	if linkType != "" && linkType != "public" && linkType != "invite" { return nil, 0, ErrInvalidGroupLibraryQuery }
	allowed := map[string]bool{"pending":true,"valid":true,"invalid":true,"approval_required":true,"unknown":true}
	if status != "" && !allowed[status] { return nil, 0, ErrInvalidGroupLibraryQuery }
	if limit < 1 || limit > 100 { limit = 50 }
	if offset < 0 { offset = 0 }
	items := []GroupLinkItem{}
	total := 0
	err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		filter := "($1='' OR l.link_type=$1) AND ($2='' OR l.status=$2) AND EXISTS (SELECT 1 FROM public_group_link_sources tenant_src WHERE tenant_src.link_id=l.id AND tenant_src.tenant_id=$3)"
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM public_group_links l WHERE "+filter, linkType, status, tenantID).Scan(&total); err != nil { return err }
		rows, err := tx.QueryContext(ctx, "SELECT l.id,l.group_id,l.link_type,COALESCE(l.public_url,''),l.link_type='invite',l.status,(SELECT COUNT(*) FROM public_group_link_sources src WHERE src.link_id=l.id AND src.tenant_id=$3),l.first_seen_at,l.last_seen_at,l.last_checked_at,COALESCE(l.last_check_error,''),l.check_attempts,l.next_check_at FROM public_group_links l WHERE "+filter+" ORDER BY l.last_seen_at DESC,l.id DESC LIMIT $4 OFFSET $5", linkType, status, tenantID, limit, offset)
		if err != nil { return err }
		defer rows.Close()
		for rows.Next() {
			var item GroupLinkItem
			if err := rows.Scan(&item.ID,&item.GroupID,&item.Type,&item.PublicURL,&item.PrivateInvite,&item.Status,&item.SourceCount,&item.FirstSeenAt,&item.LastSeenAt,&item.LastCheckedAt,&item.LastCheckError,&item.CheckAttempts,&item.NextCheckAt); err != nil { return err }
			items = append(items,item)
		}
		return rows.Err()
	})
	return items,total,err
}



type GroupLinkStats struct {
    Total int `json:"total"`
    Public int `json:"public"`
    Invite int `json:"invite"`
    Pending int `json:"pending"`
    Valid int `json:"valid"`
    ApprovalRequired int `json:"approval_required"`
    Invalid int `json:"invalid"`
    Unknown int `json:"unknown"`
}

func (s *Service) GetGroupLinkStats(ctx context.Context, tenantID string) (GroupLinkStats, error) {
    var stats GroupLinkStats
    err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
        return tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT l.id),
            COUNT(DISTINCT l.id) FILTER (WHERE l.link_type='public'),
            COUNT(DISTINCT l.id) FILTER (WHERE l.link_type='invite'),
            COUNT(DISTINCT l.id) FILTER (WHERE l.status='pending'),
            COUNT(DISTINCT l.id) FILTER (WHERE l.status='valid'),
            COUNT(DISTINCT l.id) FILTER (WHERE l.status='approval_required'),
            COUNT(DISTINCT l.id) FILTER (WHERE l.status='invalid'),
            COUNT(DISTINCT l.id) FILTER (WHERE l.status='unknown')
            FROM public_group_links l JOIN public_group_link_sources src ON src.link_id=l.id
            WHERE src.tenant_id=$1`, tenantID).Scan(&stats.Total,&stats.Public,&stats.Invite,&stats.Pending,&stats.Valid,&stats.ApprovalRequired,&stats.Invalid,&stats.Unknown)
    })
    return stats, err
}

type PublicLinkCheck struct {
 LinkID int64
 Username string
 LeaseUntil time.Time
}

func (s *Service) ListPublicLinkChecks(ctx context.Context, tenantID, accountID string, limit int) ([]PublicLinkCheck, error) {
 if !validUUID(accountID) { return nil, errors.New("invalid Telegram account id") }
 if limit < 1 || limit > 100 { limit = 50 }
 out := []PublicLinkCheck{}
 err := withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx) error {
  rows,err:=tx.QueryContext(ctx,`WITH due AS (
    SELECT l.id
    FROM public_group_links l
    WHERE l.link_type='public'
      AND EXISTS (SELECT 1 FROM public_group_link_sources src WHERE src.link_id=l.id AND src.tenant_id=$1 AND src.telegram_account_id=$2)
      AND (l.next_check_at IS NULL OR l.next_check_at<=NOW())
      AND (l.check_lease_until IS NULL OR l.check_lease_until<=NOW())
    ORDER BY COALESCE(l.last_checked_at,l.first_seen_at),l.id
    LIMIT $3 FOR UPDATE OF l SKIP LOCKED
   ), claimed AS (
    UPDATE public_group_links l
    SET check_lease_until=NOW()+INTERVAL '6 minutes',check_lease_tenant_id=$1,check_lease_account_id=$2
    FROM due WHERE l.id=due.id
    RETURNING l.id,l.public_url,l.check_lease_until
   )
   SELECT id,public_url,check_lease_until FROM claimed ORDER BY id`,tenantID,accountID,limit)
  if err!=nil{return err}
  defer rows.Close()
  for rows.Next(){
   var item PublicLinkCheck
   var raw string
   if err:=rows.Scan(&item.LinkID,&raw,&item.LeaseUntil);err!=nil{return err}
   parsed,err:=url.Parse(raw)
   if err!=nil {continue}
   parts:=strings.Split(strings.Trim(parsed.Path,"/"),"/")
   if len(parts)==0||parts[0]=="" {continue}
   item.Username=strings.TrimPrefix(parts[0],"@")
   out=append(out,item)
  }
  return rows.Err()
 })
 return out,err
}

func (s *Service) RecordPublicLinkCheck(ctx context.Context,tenantID,accountID string,linkID int64,status,checkError string,leaseUntil time.Time,retryAfter ...time.Duration) error {
 allowed:=map[string]bool{"valid":true,"invalid":true,"unknown":true}
 if !validUUID(accountID)||linkID<=0||leaseUntil.IsZero()||!allowed[status]{return errors.New("invalid public link check result")}
 if len(checkError)>500{checkError=checkError[:500]}
 retrySeconds:=int64(0);if len(retryAfter)>0&&retryAfter[0]>0{retrySeconds=int64(retryAfter[0]/time.Second);if retryAfter[0]%time.Second!=0{retrySeconds++}}
 return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
  result,err:=tx.ExecContext(ctx,`UPDATE public_group_links l SET status=$5,last_checked_at=NOW(),last_check_error=NULLIF($6,''),
    check_attempts=check_attempts+1,
    next_check_at=CASE WHEN $5='unknown' AND $7>0 THEN NOW()+GREATEST($7*INTERVAL '1 second',INTERVAL '15 minutes') WHEN $5='unknown' THEN NOW()+LEAST(INTERVAL '6 hours',INTERVAL '15 minutes' * POWER(2,LEAST(check_attempts,5))) ELSE NOW()+INTERVAL '6 hours' END,
    check_lease_until=NULL,check_lease_tenant_id=NULL,check_lease_account_id=NULL
   WHERE l.id=$1 AND l.link_type='public' AND l.check_lease_tenant_id=$2 AND l.check_lease_account_id=$3 AND l.check_lease_until=$4
    AND EXISTS(SELECT 1 FROM public_group_link_sources src WHERE src.link_id=l.id AND src.tenant_id=$2 AND src.telegram_account_id=$3)`,linkID,tenantID,accountID,leaseUntil,status,checkError,retrySeconds)
  if err!=nil{return err}
  n,_:=result.RowsAffected()
  if n==0{return errors.New("public Telegram link check lease expired or was superseded")}
  return nil
 })
}

type PrivateInviteCheck struct {
 LinkID int64
 Hash string
 CheckError string
 LeaseUntil time.Time
}

func (s *Service) ListPrivateInviteChecks(ctx context.Context, tenantID, accountID string, limit int) ([]PrivateInviteCheck, error) {
 if !validUUID(accountID) { return nil, errors.New("invalid Telegram account id") }
 if limit < 1 || limit > 100 { limit = 50 }
 out := []PrivateInviteCheck{}
 err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
  rows, err := tx.QueryContext(ctx, `WITH due AS (
    SELECT l.id
    FROM public_group_links l
    WHERE l.link_type='invite'
      AND EXISTS (SELECT 1 FROM public_group_link_sources src WHERE src.link_id=l.id AND src.tenant_id=$1 AND src.telegram_account_id=$2)
      AND (l.next_check_at IS NULL OR l.next_check_at<=NOW())
      AND (l.check_lease_until IS NULL OR l.check_lease_until<=NOW())
    ORDER BY COALESCE(l.last_checked_at,l.first_seen_at),l.id
    LIMIT $3 FOR UPDATE OF l SKIP LOCKED
   ), claimed AS (
    UPDATE public_group_links l
    SET check_lease_until=NOW()+INTERVAL '6 minutes',check_lease_tenant_id=$1,check_lease_account_id=$2
    FROM due WHERE l.id=due.id
    RETURNING l.id,l.invite_ciphertext,l.check_lease_until
   )
   SELECT id,invite_ciphertext,check_lease_until FROM claimed ORDER BY id`, tenantID, accountID, limit)
  if err != nil { return err }
  defer rows.Close()
  for rows.Next() {
   var item PrivateInviteCheck
   var ciphertext []byte
   if err := rows.Scan(&item.LinkID,&ciphertext,&item.LeaseUntil); err != nil { return err }
   hash, err := decryptJoinInviteHash(ciphertext)
   if err != nil { item.CheckError = "decrypt private invite failed: " + err.Error() } else { item.Hash = hash }
   out = append(out,item)
  }
  return rows.Err()
 })
 return out,err
}

func (s *Service) RecordPrivateInviteCheck(ctx context.Context,tenantID,accountID string,linkID int64,status,checkError string,leaseUntil time.Time,retryAfter ...time.Duration) error {
 allowed:=map[string]bool{"valid":true,"approval_required":true,"invalid":true,"unknown":true}
 if !validUUID(accountID)||linkID<=0||leaseUntil.IsZero()||!allowed[status]{return errors.New("invalid private invite check result")}
 if len(checkError)>500{checkError=checkError[:500]}
 retrySeconds:=int64(0);if len(retryAfter)>0&&retryAfter[0]>0{retrySeconds=int64(retryAfter[0]/time.Second);if retryAfter[0]%time.Second!=0{retrySeconds++}}
 return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
  result,err:=tx.ExecContext(ctx,`UPDATE public_group_links l SET status=$5,last_checked_at=NOW(),last_check_error=NULLIF($6,''),
    check_attempts=check_attempts+1,
    next_check_at=CASE WHEN $5='unknown' AND $7>0 THEN NOW()+GREATEST($7*INTERVAL '1 second',INTERVAL '15 minutes') WHEN $5='unknown' THEN NOW()+LEAST(INTERVAL '6 hours',INTERVAL '15 minutes' * POWER(2,LEAST(check_attempts,5))) ELSE NOW()+INTERVAL '6 hours' END,
    check_lease_until=NULL,check_lease_tenant_id=NULL,check_lease_account_id=NULL
   WHERE l.id=$1 AND l.link_type='invite' AND l.check_lease_tenant_id=$2 AND l.check_lease_account_id=$3 AND l.check_lease_until=$4
    AND EXISTS(SELECT 1 FROM public_group_link_sources src WHERE src.link_id=l.id AND src.tenant_id=$2 AND src.telegram_account_id=$3)`,linkID,tenantID,accountID,leaseUntil,status,checkError,retrySeconds)
  if err!=nil{return err}
  n,_:=result.RowsAffected()
  if n==0{return errors.New("private Telegram invite check lease expired or was superseded")}
  return nil
 })
}


var ErrNoReplyJob = errors.New("no pending reply job")

type ReplyJob struct {
	ID string `json:"id"`
	TenantID string `json:"tenant_id"`
	AccountID string `json:"account_id"`
	ChatID int64 `json:"chat_id"`
	ReplyToMessageID int64 `json:"reply_to_message_id"`
	Text string `json:"text"`
	MessageType string `json:"message_type"`
	ContactPhone string `json:"contact_phone,omitempty"`
	ContactFirstName string `json:"contact_first_name,omitempty"`
	ContactLastName string `json:"contact_last_name,omitempty"`
	Attempts int `json:"attempts"`
	Status string `json:"status"`
	TelegramMessageID int64 `json:"telegram_message_id,omitempty"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	LeaseUntil time.Time `json:"-"`
}

func (s *Service) CreateContactReply(ctx context.Context,tenantID,messageID,phone,firstName,lastName string,idempotencyKeys ...string)(ReplyJob,error) {
	phone=strings.TrimSpace(phone);firstName=strings.TrimSpace(firstName);lastName=strings.TrimSpace(lastName)
	if !validUUID(messageID)||len(phone)<3||len(phone)>32||len(firstName)<1||len(firstName)>64||len(lastName)>64||strings.ContainsAny(phone,"\\r\\n") {
		return ReplyJob{},errors.New("contact requires a valid message id, phone, first name, and optional last name")
	}
	key:=""
	if len(idempotencyKeys)>0 { key=idempotencyKeys[0] }
	var err error
	if key,err=normalizeIdempotencyKey(key);err!=nil { return ReplyJob{},err }
	requestHash:=""
	if key!="" { requestHash=replyRequestHash("contact_reply",messageID,phone,firstName,lastName) }
	var job ReplyJob
	err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		err:=tx.QueryRowContext(ctx,`SELECT telegram_account_id::text,chat_id,message_id FROM unified_messages WHERE tenant_id=$1 AND id=$2`,tenantID,messageID).Scan(&job.AccountID,&job.ChatID,&job.ReplyToMessageID)
		if errors.Is(err,sql.ErrNoRows){return errors.New("message not found")};if err!=nil{return err}
		if key=="" {
			err=tx.QueryRowContext(ctx,`INSERT INTO message_reply_jobs(tenant_id,unified_message_id,telegram_account_id,chat_id,reply_to_message_id,message_text,message_type,contact_phone,contact_first_name,contact_last_name)
				VALUES($1,$2,$3,$4,$5,'','contact',$6,$7,$8) RETURNING id::text,status,attempts,created_at`,tenantID,messageID,job.AccountID,job.ChatID,job.ReplyToMessageID,phone,firstName,lastName).Scan(&job.ID,&job.Status,&job.Attempts,&job.CreatedAt)
		} else {
			err=tx.QueryRowContext(ctx,`INSERT INTO message_reply_jobs(tenant_id,unified_message_id,telegram_account_id,chat_id,reply_to_message_id,message_text,message_type,contact_phone,contact_first_name,contact_last_name,idempotency_key,request_hash)
				VALUES($1,$2,$3,$4,$5,'','contact',$6,$7,$8,$9,$10)
				ON CONFLICT(tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
				RETURNING id::text,status,attempts,created_at`,tenantID,messageID,job.AccountID,job.ChatID,job.ReplyToMessageID,phone,firstName,lastName,key,requestHash).Scan(&job.ID,&job.Status,&job.Attempts,&job.CreatedAt)
			if errors.Is(err,sql.ErrNoRows) {
				var existingHash string
				err=tx.QueryRowContext(ctx,`SELECT id::text,status,attempts,created_at,request_hash,telegram_account_id::text,chat_id,reply_to_message_id,contact_phone,contact_first_name,contact_last_name
					FROM message_reply_jobs WHERE tenant_id=$1 AND idempotency_key=$2`,tenantID,key).
					Scan(&job.ID,&job.Status,&job.Attempts,&job.CreatedAt,&existingHash,&job.AccountID,&job.ChatID,&job.ReplyToMessageID,&job.ContactPhone,&job.ContactFirstName,&job.ContactLastName)
				if err==nil && existingHash!=requestHash { return errors.New("Idempotency-Key was already used with a different contact request") }
			}
		}
		if err==nil {
			job.TenantID=tenantID;job.MessageType="contact"
			if job.ContactPhone=="" {job.ContactPhone=phone;job.ContactFirstName=firstName;job.ContactLastName=lastName}
		}
		return err
	})
	return job,err
}

func normalizeIdempotencyKey(key string) (string,error) {
	key=strings.TrimSpace(key)
	if key=="" { return "",nil }
	if len(key)<8 || len(key)>128 || strings.IndexFunc(key,unicode.IsControl)>=0 {
		return "",errors.New("Idempotency-Key must contain 8-128 non-control characters")
	}
	return key,nil
}

func replyRequestHash(operation string, fields ...string) string {
	parts:=append([]string{operation},fields...)
	sum:=sha256.Sum256([]byte(strings.Join(parts,"\x00")))
	return hex.EncodeToString(sum[:])
}

func (s *Service) CreateReply(ctx context.Context, tenantID, messageID, textBody string, idempotencyKeys ...string) (ReplyJob,error) {
	textBody=strings.TrimSpace(textBody)
	if !validUUID(messageID) || textBody=="" || len([]rune(textBody))>4096 {
		return ReplyJob{},errors.New("reply text must contain 1-4096 characters and message id must be valid")
	}
	key:=""
	if len(idempotencyKeys)>0 { key=idempotencyKeys[0] }
	var err error
	if key,err=normalizeIdempotencyKey(key);err!=nil { return ReplyJob{},err }
	requestHash:=""
	if key!="" { requestHash=replyRequestHash("message_reply",messageID,textBody) }
	var job ReplyJob
	err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		err:=tx.QueryRowContext(ctx,`SELECT telegram_account_id::text,chat_id,message_id FROM unified_messages WHERE tenant_id=$1 AND id=$2`,tenantID,messageID).Scan(&job.AccountID,&job.ChatID,&job.ReplyToMessageID)
		if errors.Is(err,sql.ErrNoRows){return errors.New("message not found")};if err!=nil{return err}
		if key=="" {
			err=tx.QueryRowContext(ctx,`INSERT INTO message_reply_jobs(tenant_id,unified_message_id,telegram_account_id,chat_id,reply_to_message_id,message_text) VALUES($1,$2,$3,$4,$5,$6) RETURNING id::text,status,attempts,created_at`,tenantID,messageID,job.AccountID,job.ChatID,job.ReplyToMessageID,textBody).Scan(&job.ID,&job.Status,&job.Attempts,&job.CreatedAt)
		} else {
			err=tx.QueryRowContext(ctx,`INSERT INTO message_reply_jobs(tenant_id,unified_message_id,telegram_account_id,chat_id,reply_to_message_id,message_text,idempotency_key,request_hash)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8)
				ON CONFLICT(tenant_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO NOTHING
				RETURNING id::text,status,attempts,created_at`,tenantID,messageID,job.AccountID,job.ChatID,job.ReplyToMessageID,textBody,key,requestHash).Scan(&job.ID,&job.Status,&job.Attempts,&job.CreatedAt)
			if errors.Is(err,sql.ErrNoRows) {
					var existingHash string
					err=tx.QueryRowContext(ctx,`SELECT id::text,status,attempts,created_at,request_hash,telegram_account_id::text,chat_id,reply_to_message_id,message_text
						FROM message_reply_jobs WHERE tenant_id=$1 AND idempotency_key=$2`,tenantID,key).
						Scan(&job.ID,&job.Status,&job.Attempts,&job.CreatedAt,&existingHash,&job.AccountID,&job.ChatID,&job.ReplyToMessageID,&job.Text)
					if err==nil && existingHash!=requestHash { return errors.New("Idempotency-Key was already used with a different reply request") }
			}
		}
		if err==nil {
			job.TenantID=tenantID
			if job.Text=="" { job.Text=textBody }
		}
		return err
	})
	return job,err
}

// ClaimPendingReplies claims due jobs under each tenant's RLS scope. Expired
// in-flight sends are quarantined instead of retried because their Telegram
// outcome may be ambiguous after a worker crash.
func (s *Service) ClaimPendingReplies(ctx context.Context, maxJobs int) ([]ReplyJob,error) {
	if maxJobs<1||maxJobs>50 {maxJobs=10}
	// A lease expiring while a send was in flight has an ambiguous Telegram
	// outcome: Telegram may have accepted the message just before the worker died.
	// Never automatically resend that job. Freeze it for operator verification;
	// an explicit ResumeReply remains available after checking the destination chat.
	uncertainRows,err:=s.db.QueryContext(ctx,`SELECT DISTINCT tenant_id::text FROM message_reply_jobs
		WHERE status='processing' AND lease_until<=NOW()`)
	if err!=nil{return nil,err}
	var uncertainTenants []string
	for uncertainRows.Next(){var id string;if err:=uncertainRows.Scan(&id);err!=nil{uncertainRows.Close();return nil,err};uncertainTenants=append(uncertainTenants,id)}
	if err:=uncertainRows.Err();err!=nil{uncertainRows.Close();return nil,err}
	uncertainRows.Close()
	for _,tenantID:=range uncertainTenants {
		if err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
			_,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status='paused',lease_until=NULL,
				last_error='send outcome uncertain after worker lease expired; verify Telegram chat before resuming',
				updated_at=NOW()
				WHERE tenant_id=$1 AND status='processing' AND lease_until<=NOW()`,tenantID)
			return err
		});err!=nil{return nil,err}
	}
	// Expired/disabled tenants are swept before any active tenant can claim work.
	inactiveRows,err:=s.db.QueryContext(ctx,`SELECT id::text FROM tenants WHERE status<>'active' OR expires_at IS NULL OR expires_at<=NOW()`)
	if err!=nil{return nil,err}
	var inactiveTenants []string
	for inactiveRows.Next(){var id string;if err:=inactiveRows.Scan(&id);err!=nil{inactiveRows.Close();return nil,err};inactiveTenants=append(inactiveTenants,id)}
	if err:=inactiveRows.Err();err!=nil{inactiveRows.Close();return nil,err};inactiveRows.Close()
	for _,tenantID:=range inactiveTenants {
		if err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
			// Preserve a live processing lease while a Telegram send is in flight.
			// Expired processing leases are safe to pause after worker recovery.
			_,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status='paused',lease_until=NULL,last_error='tenant inactive; manual review required',updated_at=NOW()
				WHERE tenant_id=$1 AND (status='pending' OR (status='processing' AND lease_until<=NOW()))`,tenantID)
			return err
		});err!=nil{return nil,err}
	}
	jobs:=make([]ReplyJob,0,maxJobs)
	// Bound scheduler probes: a concurrent Worker may hold a selected job/account
	// lock after the tenant cursor is advanced, yielding no claim this pass.
	// Never spin indefinitely when all eligible work is temporarily locked.
	maxTenantProbes:=maxJobs*10
	if maxTenantProbes<10 { maxTenantProbes=10 }
	for probes:=0; len(jobs)<maxJobs && probes<maxTenantProbes; probes++ {
		tenantID,err:=s.nextReplyTenant(ctx)
		if err!=nil{return nil,err}
		if tenantID==""{break}
		err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
			remaining:=maxJobs-len(jobs)
			if remaining>2 { remaining=2 }
			rows,err:=tx.QueryContext(ctx,`WITH picked AS (
				SELECT j.id,j.tenant_id,j.telegram_account_id
				FROM message_reply_jobs j JOIN telegram_accounts a ON a.tenant_id=j.tenant_id AND a.id=j.telegram_account_id
				WHERE j.tenant_id=$1
				AND EXISTS (SELECT 1 FROM tenants active_tenant WHERE active_tenant.id=j.tenant_id AND active_tenant.status='active' AND active_tenant.expires_at>NOW())
				AND j.status='pending' AND j.available_at<=NOW()
				AND a.status='authorized' AND a.session_ciphertext IS NOT NULL
				AND COALESCE(a.telegram_send_next_at,'-infinity'::timestamptz)<=NOW()
				AND COALESCE(a.telegram_send_cooldown_until,'-infinity'::timestamptz)<=NOW()
				AND NOT EXISTS (SELECT 1 FROM message_reply_jobs x WHERE x.tenant_id=j.tenant_id AND x.telegram_account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
				AND NOT EXISTS (SELECT 1 FROM keyword_action_jobs x WHERE x.tenant_id=j.tenant_id AND x.telegram_account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
				AND NOT EXISTS (SELECT 1 FROM tenant_join_task_accounts x WHERE x.tenant_id=j.tenant_id AND x.telegram_account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
				AND NOT EXISTS (SELECT 1 FROM account_profile_update_jobs x WHERE x.tenant_id=j.tenant_id AND x.account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
				AND NOT EXISTS (SELECT 1 FROM message_reply_jobs e WHERE e.tenant_id=j.tenant_id AND e.telegram_account_id=j.telegram_account_id
				AND e.status='pending' AND e.available_at<=NOW() AND (e.created_at,e.id)<(j.created_at,j.id))
				ORDER BY j.created_at,j.id FOR UPDATE OF j,a SKIP LOCKED LIMIT $2
				), claimed AS (
				UPDATE message_reply_jobs j SET status='processing',attempts=j.attempts+1,lease_until=NOW()+INTERVAL '5 minutes',updated_at=NOW()
				FROM picked p WHERE j.id=p.id AND j.tenant_id=p.tenant_id
				RETURNING j.id::text,j.tenant_id::text,j.telegram_account_id::text,j.chat_id,j.reply_to_message_id,j.message_text,j.message_type,j.contact_phone,j.contact_first_name,j.contact_last_name,j.attempts,j.status,j.created_at,j.lease_until
				), scheduled AS (
				UPDATE telegram_accounts a SET telegram_send_next_at=NOW()+INTERVAL '2 seconds'
				FROM claimed c WHERE a.tenant_id=c.tenant_id::uuid AND a.id=c.telegram_account_id::uuid RETURNING a.id
				) SELECT c.* FROM claimed c JOIN scheduled s ON s.id=c.telegram_account_id::uuid`,tenantID,remaining)
			if err!=nil{return err}
			defer rows.Close()
			for rows.Next(){var job ReplyJob;if err:=rows.Scan(&job.ID,&job.TenantID,&job.AccountID,&job.ChatID,&job.ReplyToMessageID,&job.Text,&job.MessageType,&job.ContactPhone,&job.ContactFirstName,&job.ContactLastName,&job.Attempts,&job.Status,&job.CreatedAt,&job.LeaseUntil);err!=nil{return err};jobs=append(jobs,job)}
			return rows.Err()
		})
		if err!=nil{return nil,err}
	}
	return jobs,nil
}

// nextReplyTenant atomically rotates the next eligible tenant. The row lock
// prevents multiple Worker instances from repeatedly selecting the same tenant.
func (s *Service) nextReplyTenant(ctx context.Context) (string,error) {
	tx,err:=s.db.BeginTx(ctx,nil)
	if err!=nil{return "",err}
	defer tx.Rollback()
	var tenantID string
	err=tx.QueryRowContext(ctx,`WITH candidate AS (
		SELECT t.id
		FROM tenants t
		WHERE t.status='active' AND t.expires_at>NOW()
		AND EXISTS (
			SELECT 1 FROM message_reply_jobs j
			JOIN telegram_accounts a ON a.tenant_id=j.tenant_id AND a.id=j.telegram_account_id
			WHERE j.tenant_id=t.id
			AND j.status='pending' AND j.available_at<=NOW()
			AND a.status='authorized' AND a.session_ciphertext IS NOT NULL
			AND COALESCE(a.telegram_send_next_at,'-infinity'::timestamptz)<=NOW()
			AND COALESCE(a.telegram_send_cooldown_until,'-infinity'::timestamptz)<=NOW()
			AND NOT EXISTS (SELECT 1 FROM message_reply_jobs x WHERE x.tenant_id=j.tenant_id AND x.telegram_account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
			AND NOT EXISTS (SELECT 1 FROM keyword_action_jobs x WHERE x.tenant_id=j.tenant_id AND x.telegram_account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
			AND NOT EXISTS (SELECT 1 FROM tenant_join_task_accounts x WHERE x.tenant_id=j.tenant_id AND x.telegram_account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
			AND NOT EXISTS (SELECT 1 FROM account_profile_update_jobs x WHERE x.tenant_id=j.tenant_id AND x.account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
			AND NOT EXISTS (SELECT 1 FROM message_reply_jobs e WHERE e.tenant_id=j.tenant_id AND e.telegram_account_id=j.telegram_account_id
				AND e.status='pending' AND e.available_at<=NOW() AND (e.created_at,e.id)<(j.created_at,j.id))
		)
		ORDER BY t.reply_jobs_last_claimed_at NULLS FIRST,t.id
		LIMIT 1
		FOR UPDATE OF t SKIP LOCKED
	)
	UPDATE tenants t SET reply_jobs_last_claimed_at=clock_timestamp()
	FROM candidate c WHERE t.id=c.id
	RETURNING t.id::text`).Scan(&tenantID)
	if errors.Is(err,sql.ErrNoRows){return "",nil}
	if err!=nil{return "",err}
	if err:=tx.Commit();err!=nil{return "",err}
	return tenantID,nil
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
		_,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status='paused',lease_until=NULL,last_error=$3,updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$4`,job.TenantID,job.ID,reason,job.LeaseUntil)
		return err
	})
}

func (s *Service) CompleteReply(ctx context.Context, job ReplyJob, telegramMessageID int64) error {
	if telegramMessageID<=0{return errors.New("invalid Telegram message id")}
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		// Persist the Telegram result and the outbound conversation message in
		// one transaction. If the worker crashes after Telegram accepts the send,
		// a later retry of this DB transaction is safe and does not send again.
		var accountID,sourceMessageID string
		var chatID,replyToID int64
		var textBody,messageType string
		err:=tx.QueryRowContext(ctx,`SELECT telegram_account_id::text,unified_message_id::text,chat_id,reply_to_message_id,message_text,message_type
			FROM message_reply_jobs WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$3
			FOR UPDATE`,job.TenantID,job.ID,job.LeaseUntil).Scan(&accountID,&sourceMessageID,&chatID,&replyToID,&textBody,&messageType)
		if errors.Is(err,sql.ErrNoRows){return errors.New("reply job lease not found")}
		if err!=nil{return err}
		res,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status='sent',telegram_message_id=$3,last_error='',lease_until=NULL,updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$4`,job.TenantID,job.ID,telegramMessageID,job.LeaseUntil)
		if err!=nil{return err}
		n,_:=res.RowsAffected();if n==0{return errors.New("reply job lease not found")}
		mediaType:=""
		if messageType=="contact"{mediaType="contact"}
		_,err=tx.ExecContext(ctx,`INSERT INTO unified_messages(
			tenant_id,telegram_account_id,chat_id,message_id,chat_type,chat_title,sender_id,
			reply_to_message_id,reply_to_own_message,is_outgoing,message_text,media_type,message_date
		)
		SELECT $1,$2,$3,$4,source.chat_type,source.chat_title,0,$5,TRUE,TRUE,$6,$7,NOW()
		FROM unified_messages source
		WHERE source.tenant_id=$1 AND source.id=$8
		ON CONFLICT(telegram_account_id,chat_id,message_id) DO NOTHING`,
			job.TenantID,accountID,chatID,telegramMessageID,replyToID,textBody,mediaType,sourceMessageID)
		if err!=nil{return err}
		payload,err:=json.Marshal(MessageCreatedEvent{Type:"message.created",AccountID:accountID,ChatID:chatID,MessageID:telegramMessageID,CreatedAt:time.Now().UTC()})
		if err!=nil{return err}
		_,err=tx.ExecContext(ctx,`INSERT INTO message_event_outbox(tenant_id,event_type,payload)
			VALUES($1,'message.created',$2::jsonb)`,job.TenantID,string(payload))
		return err
	})
}

func replyRetrySchedule(attempts int, floodWait time.Duration) (string, int64) {
	if floodWait > 0 {
		seconds := int64(floodWait / time.Second)
		if floodWait%time.Second != 0 { seconds++ }
		return "pending", seconds + 1
	}
	status := "pending"
	if attempts >= 5 { status = "failed" }
	delay := attempts * 30
	if delay > 900 { delay = 900 }
	if delay < 30 { delay = 30 }
	return status, int64(delay)
}

func (s *Service) FailReply(ctx context.Context, job ReplyJob, cause error) error {
	return s.FailReplyWithFloodWait(ctx, job, cause, 0)
}

// FailReplyWithFloodWait preserves Telegram's server-mandated cooldown. A
// FloodWait never consumes the normal retry budget or triggers account rotation.
func (s *Service) FailReplyWithFloodWait(ctx context.Context, job ReplyJob, cause error, floodWait time.Duration) error {
	if cause==nil{cause=errors.New("unknown Telegram send error")}
	message:=cause.Error();if len(message)>500{message=message[:500]}
	status,delay:=replyRetrySchedule(job.Attempts,floodWait)
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		var tenantActive bool
		if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1 AND status='active' AND expires_at>NOW())`,job.TenantID).Scan(&tenantActive);err!=nil{return err}
		if !tenantActive {
			res,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status='paused',last_error='tenant inactive; manual review required',lease_until=NULL,updated_at=NOW()
				WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$3`,job.TenantID,job.ID,job.LeaseUntil)
			if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("reply job lease not found")}
			if floodWait>0 {
				if _,err:=tx.ExecContext(ctx,`UPDATE telegram_accounts SET telegram_send_cooldown_until=GREATEST(COALESCE(telegram_send_cooldown_until,NOW()),NOW()+($1 * INTERVAL '1 second')) WHERE tenant_id=$2 AND id=$3`,delay,job.TenantID,job.AccountID);err!=nil{return err}
			}
			return nil
		}
		res,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status=$3,last_error=$4,available_at=NOW()+($5 * INTERVAL '1 second'),lease_until=NULL,updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$6`,job.TenantID,job.ID,status,message,delay,job.LeaseUntil)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("reply job lease not found")}
		if floodWait>0 { if _,err:=tx.ExecContext(ctx,`UPDATE telegram_accounts SET telegram_send_cooldown_until=GREATEST(COALESCE(telegram_send_cooldown_until,NOW()),NOW()+($1 * INTERVAL '1 second')) WHERE tenant_id=$2 AND id=$3`,delay,job.TenantID,job.AccountID);err!=nil{return err} }
		return nil
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
		res,err:=tx.ExecContext(ctx,`UPDATE message_reply_jobs SET status='pending',attempts=GREATEST(attempts-1,0),available_at=NOW()+($3 * INTERVAL '1 second'),lease_until=NULL,updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$4`,job.TenantID,job.ID,int(delay.Seconds()),job.LeaseUntil)
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
