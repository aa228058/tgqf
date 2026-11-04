package messages

import (
    "context"
    "database/sql"
    "encoding/json"
    "errors"
    "fmt"
    "regexp"
    "strings"
    "time"
)

var ErrInvalidKeywordRule = errors.New("invalid keyword rule")

type KeywordMediaAlbumItem struct { AssetID string `json:"asset_id"`; MediaType string `json:"media_type"`; Caption string `json:"caption,omitempty"` }

type KeywordRule struct {
    KeywordGroups [][]string `json:"keyword_groups"`
    GroupOperator string `json:"group_operator"`
    ID string `json:"id"`
    Name string `json:"name"`
    MatchMode string `json:"match_mode"`
    Keywords []string `json:"keywords"`
    ExcludeTerms []string `json:"exclude_terms"`
    ChatTypes []string `json:"chat_types"`
    AccountIDs []string `json:"account_ids"`
    ChatIDs []int64 `json:"chat_ids"`
    MediaTypes []string `json:"media_types"`
    ChatTitles []string `json:"chat_titles"`
    ChatUsernames []string `json:"chat_usernames"`
    SenderIDs []int64 `json:"sender_ids"`
    SenderUsernames []string `json:"sender_usernames"`
    SenderNames []string `json:"sender_names"`
    ReplyText string `json:"reply_text"`
    ActionType string `json:"action_type"`
    ContactPhone string `json:"contact_phone"`
    ContactFirstName string `json:"contact_first_name"`
    ContactLastName string `json:"contact_last_name"`
    MediaAssetID string `json:"media_asset_id,omitempty"`
    MediaType string `json:"media_type,omitempty"`
    MediaCaption string `json:"media_caption,omitempty"`
    MediaAlbumItems []KeywordMediaAlbumItem `json:"media_album_items,omitempty"`
    ReplyTextByChatType map[string]string `json:"reply_text_by_chat_type"`
    CaseSensitive bool `json:"case_sensitive"`
    Enabled bool `json:"enabled"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type KeywordMatch struct {
    ID int64 `json:"id"`
    RuleID string `json:"rule_id"`
    RuleName string `json:"rule_name"`
    AccountID string `json:"account_id"`
    ChatID int64 `json:"chat_id"`
    MessageID int64 `json:"message_id"`
    MatchedTerms []string `json:"matched_terms"`
    MatchedAt time.Time `json:"matched_at"`
}

func normalizeKeywordRule(rule KeywordRule) (KeywordRule, error) {
    rule.Name = strings.TrimSpace(rule.Name)
    rule.MatchMode = strings.ToLower(strings.TrimSpace(rule.MatchMode))
    if len(rule.Name)==0 || len(rule.Name)>80 { return rule, fmt.Errorf("%w: name must be 1-80 characters",ErrInvalidKeywordRule) }
    if rule.MatchMode!="any" && rule.MatchMode!="all" && rule.MatchMode!="exact" && rule.MatchMode!="regex" { return rule, fmt.Errorf("%w: match_mode must be any, all, exact, or regex",ErrInvalidKeywordRule) }
    if len(rule.Keywords)>20 || len(rule.ExcludeTerms)>20 { return rule, fmt.Errorf("%w: provide at most 20 keywords and 20 exclude terms",ErrInvalidKeywordRule) }
    normalizeTerms:=func(items []string, allowRegex bool)([]string,error){
        out:=make([]string,0,len(items));seen:=map[string]bool{}
        for _,item:=range items {
            item=strings.TrimSpace(item)
            if item=="" || len(item)>256 { return nil,ErrInvalidKeywordRule }
            key:=item;if !rule.CaseSensitive {key=strings.ToLower(key)}
            if seen[key] {continue};seen[key]=true
            if allowRegex { if _,err:=regexp.Compile(item);err!=nil{return nil,fmt.Errorf("%w: invalid regular expression",ErrInvalidKeywordRule)} }
            out=append(out,item)
        }
        return out,nil
    }
    var err error
    rule.Keywords,err=normalizeTerms(rule.Keywords,rule.MatchMode=="regex");if err!=nil{return rule,err}
    if rule.GroupOperator=="" {rule.GroupOperator="any"}
    rule.GroupOperator=strings.ToLower(strings.TrimSpace(rule.GroupOperator))
    if rule.GroupOperator!="any"&&rule.GroupOperator!="all" {return rule,fmt.Errorf("%w: group_operator must be any or all",ErrInvalidKeywordRule)}
    if len(rule.KeywordGroups)>10 {return rule,fmt.Errorf("%w: at most 10 keyword groups are allowed",ErrInvalidKeywordRule)}
    normalizedGroups:=make([][]string,0,len(rule.KeywordGroups))
    for _,group:=range rule.KeywordGroups {if len(group)==0||len(group)>20{return rule,fmt.Errorf("%w: each keyword group must contain 1-20 terms",ErrInvalidKeywordRule)};normalized,groupErr:=normalizeTerms(group,rule.MatchMode=="regex");if groupErr!=nil{return rule,groupErr};normalizedGroups=append(normalizedGroups,normalized)}
    rule.KeywordGroups=normalizedGroups
    rule.ExcludeTerms,err=normalizeTerms(rule.ExcludeTerms,false);if err!=nil{return rule,err}
    if rule.ChatTypes==nil {rule.ChatTypes=[]string{"private","group","channel"}}
    if rule.AccountIDs==nil {rule.AccountIDs=[]string{}}
    if rule.ChatIDs==nil {rule.ChatIDs=[]int64{}}
    if rule.MediaTypes==nil {rule.MediaTypes=[]string{}}
    if rule.ChatTitles==nil {rule.ChatTitles=[]string{}}
    if rule.ChatUsernames==nil {rule.ChatUsernames=[]string{}}
    if rule.SenderIDs==nil {rule.SenderIDs=[]int64{}}
    if rule.SenderUsernames==nil {rule.SenderUsernames=[]string{}}
    if rule.SenderNames==nil {rule.SenderNames=[]string{}}
    if len(rule.AccountIDs)>100 || len(rule.ChatIDs)>1000 || len(rule.MediaTypes)>8 || len(rule.ChatTitles)>100 || len(rule.ChatUsernames)>100 || len(rule.SenderIDs)>1000 || len(rule.SenderUsernames)>100 || len(rule.SenderNames)>100 {return rule,fmt.Errorf("%w: keyword filter limits exceeded",ErrInvalidKeywordRule)}
    seenAccounts:=map[string]bool{}
    for _,id:=range rule.AccountIDs {if !validUUID(id)||seenAccounts[id] {return rule,fmt.Errorf("%w: invalid or duplicate account filter",ErrInvalidKeywordRule)};seenAccounts[id]=true}
    seenChats:=map[int64]bool{}
    for _,id:=range rule.ChatIDs {if id==0||seenChats[id] {return rule,fmt.Errorf("%w: invalid or duplicate chat filter",ErrInvalidKeywordRule)};seenChats[id]=true}
    allowedMedia:=map[string]bool{"photo":true,"video":true,"document":true,"audio":true,"voice":true,"video_note":true,"animation":true,"sticker":true}
    seenMedia:=map[string]bool{}
    for i,kind:=range rule.MediaTypes {kind=strings.ToLower(strings.TrimSpace(kind));if !allowedMedia[kind]||seenMedia[kind] {return rule,fmt.Errorf("%w: unsupported or duplicate media type",ErrInvalidKeywordRule)};seenMedia[kind]=true;rule.MediaTypes[i]=kind}
    if len(rule.ChatTypes)==0 || len(rule.ChatTypes)>3 {return rule,fmt.Errorf("%w: chat_types must include one or more supported types",ErrInvalidKeywordRule)}
    allowed:=map[string]bool{"private":true,"group":true,"channel":true};seenTypes:=map[string]bool{}
    normalizedTypes:=make([]string,0,len(rule.ChatTypes))
    for _,kind:=range rule.ChatTypes {kind=strings.ToLower(strings.TrimSpace(kind));if !allowed[kind]||seenTypes[kind]{return rule,fmt.Errorf("%w: unsupported or duplicate chat type",ErrInvalidKeywordRule)};seenTypes[kind]=true;normalizedTypes=append(normalizedTypes,kind)}
    rule.ChatTypes=normalizedTypes
    normalizeFilters:=func(items []string)([]string,error){out:=make([]string,0,len(items));seen:=map[string]bool{};for _,v:=range items{v=strings.TrimSpace(strings.TrimPrefix(v,"@"));if v==""||len(v)>256{return nil,ErrInvalidKeywordRule};key:=strings.ToLower(v);if !seen[key]{seen[key]=true;out=append(out,v)}};return out,nil}
    rule.ChatTitles,err=normalizeFilters(rule.ChatTitles);if err!=nil{return rule,err}
    rule.ChatUsernames,err=normalizeFilters(rule.ChatUsernames);if err!=nil{return rule,err}
    rule.SenderUsernames,err=normalizeFilters(rule.SenderUsernames);if err!=nil{return rule,err}
    rule.SenderNames,err=normalizeFilters(rule.SenderNames);if err!=nil{return rule,err}
    seenSender:=map[int64]bool{};for _,id:=range rule.SenderIDs{if id==0||seenSender[id]{return rule,fmt.Errorf("%w: invalid or duplicate sender id",ErrInvalidKeywordRule)};seenSender[id]=true}
    rule.ReplyText=strings.TrimSpace(rule.ReplyText)
    rule.ActionType=strings.ToLower(strings.TrimSpace(rule.ActionType)); if rule.ActionType=="" { rule.ActionType="text" }
    rule.ContactPhone=strings.TrimSpace(rule.ContactPhone); rule.ContactFirstName=strings.TrimSpace(rule.ContactFirstName); rule.ContactLastName=strings.TrimSpace(rule.ContactLastName)
    rule.MediaAssetID=strings.TrimSpace(rule.MediaAssetID); rule.MediaType=strings.ToLower(strings.TrimSpace(rule.MediaType)); rule.MediaCaption=strings.TrimSpace(rule.MediaCaption)
    if rule.ActionType!="text" && rule.ActionType!="contact" && rule.ActionType!="media" && rule.ActionType!="media_album" { return rule,fmt.Errorf("%w: action_type must be text, contact, media, or media_album",ErrInvalidKeywordRule) }
    if rule.ActionType=="contact" {
        if rule.ContactPhone=="" || rule.ContactFirstName=="" || len(rule.ContactPhone)>64 || len(rule.ContactFirstName)>128 || len(rule.ContactLastName)>128 { return rule,fmt.Errorf("%w: contact action requires phone and first name",ErrInvalidKeywordRule) }
        if rule.ReplyText!="" || len(rule.ReplyTextByChatType)>0 { return rule,fmt.Errorf("%w: contact action cannot include text replies",ErrInvalidKeywordRule) }
    } else if rule.ContactPhone!="" || rule.ContactFirstName!="" || rule.ContactLastName!="" { return rule,fmt.Errorf("%w: contact fields require contact action",ErrInvalidKeywordRule) }
    if rule.ActionType=="media_album" {
        if len(rule.MediaAlbumItems)<2||len(rule.MediaAlbumItems)>10{return rule,fmt.Errorf("%w: album must contain 2-10 items",ErrInvalidKeywordRule)}
        category:="";seen:=map[string]bool{}
        for i:=range rule.MediaAlbumItems { item:=&rule.MediaAlbumItems[i];item.AssetID=strings.TrimSpace(item.AssetID);item.MediaType=strings.ToLower(strings.TrimSpace(item.MediaType));item.Caption=strings.TrimSpace(item.Caption);if !validUUID(item.AssetID)||seen[item.AssetID]{return rule,fmt.Errorf("%w: invalid or duplicate album asset",ErrInvalidKeywordRule)};seen[item.AssetID]=true;current:="visual";if item.MediaType=="document"{current="document"}else if item.MediaType!="photo"&&item.MediaType!="video"{return rule,fmt.Errorf("%w: albums support photo/video/document",ErrInvalidKeywordRule)};if category!=""&&category!=current{return rule,fmt.Errorf("%w: cannot mix documents with photos/videos",ErrInvalidKeywordRule)};category=current;if len([]rune(item.Caption))>1024{return rule,fmt.Errorf("%w: album caption too long",ErrInvalidKeywordRule)} }
        if rule.ReplyText!=""||len(rule.ReplyTextByChatType)>0||rule.ContactPhone!=""||rule.ContactFirstName!=""||rule.ContactLastName!=""||rule.MediaAssetID!=""||rule.MediaType!=""||rule.MediaCaption!=""{return rule,fmt.Errorf("%w: album cannot mix action payloads",ErrInvalidKeywordRule)}
    } else if len(rule.MediaAlbumItems)>0{return rule,fmt.Errorf("%w: album items require media_album action",ErrInvalidKeywordRule)}
    if rule.ActionType=="media" {
        if !validUUID(rule.MediaAssetID) { return rule,fmt.Errorf("%w: media action requires a valid media_asset_id",ErrInvalidKeywordRule) }
        switch rule.MediaType { case "photo","video","document","audio","voice","video_note": default: return rule,fmt.Errorf("%w: unsupported media_type",ErrInvalidKeywordRule) }
        if len([]rune(rule.MediaCaption))>1024 { return rule,fmt.Errorf("%w: media caption exceeds Telegram's 1024 character limit",ErrInvalidKeywordRule) }
        if rule.ReplyText!="" || len(rule.ReplyTextByChatType)>0 { return rule,fmt.Errorf("%w: media action cannot include text replies",ErrInvalidKeywordRule) }
        if rule.ContactPhone!="" || rule.ContactFirstName!="" || rule.ContactLastName!="" { return rule,fmt.Errorf("%w: media action cannot include contact fields",ErrInvalidKeywordRule) }
    } else if rule.MediaAssetID!="" || rule.MediaType!="" || rule.MediaCaption!="" { return rule,fmt.Errorf("%w: media fields require media action",ErrInvalidKeywordRule) }
    if rule.ActionType=="contact" || rule.ActionType=="media" || rule.ActionType=="media_album" {
        for _,chatType:=range rule.ChatTypes { if chatType=="channel" { return rule,fmt.Errorf("%w: contact actions support private and group chats only",ErrInvalidKeywordRule) } }
    }
    if len([]rune(rule.ReplyText))>4096{return rule,fmt.Errorf("%w: reply text exceeds Telegram's 4096 character limit",ErrInvalidKeywordRule)}
    if rule.ReplyTextByChatType==nil {rule.ReplyTextByChatType=map[string]string{}}
    normalizedReplies:=make(map[string]string,len(rule.ReplyTextByChatType))
    for chatType,reply:=range rule.ReplyTextByChatType {
        chatType=strings.ToLower(strings.TrimSpace(chatType))
        if chatType!="private"&&chatType!="group" {return rule,fmt.Errorf("%w: reply_text_by_chat_type supports only private and group",ErrInvalidKeywordRule)}
        if _,exists:=normalizedReplies[chatType];exists {return rule,fmt.Errorf("%w: duplicate chat-specific reply type",ErrInvalidKeywordRule)}
        reply=strings.TrimSpace(reply)
        if len([]rune(reply))>4096{return rule,fmt.Errorf("%w: chat-specific reply exceeds Telegram's 4096 character limit",ErrInvalidKeywordRule)}
        normalizedReplies[chatType]=reply
    }
    rule.ReplyTextByChatType=normalizedReplies
    if len(rule.KeywordGroups)>0 && len(rule.Keywords)>0 {return rule,fmt.Errorf("%w: use keywords or keyword_groups, not both",ErrInvalidKeywordRule)}
    if len(rule.Keywords)==0 && len(rule.KeywordGroups)==0 && len(rule.MediaTypes)==0 {return rule,fmt.Errorf("%w: provide keywords, keyword groups, or at least one media type",ErrInvalidKeywordRule)}
    return rule,nil
}

func keywordRuleMatches(rule KeywordRule, in Incoming) ([]string,bool) {
    textPresent:=strings.TrimSpace(in.Text)!=""
    if in.IsOutgoing || (!textPresent && len(rule.MediaTypes)==0) {return nil,false}
    if len(rule.AccountIDs)>0 {found:=false;for _,id:=range rule.AccountIDs {if id==in.AccountID {found=true;break}};if !found{return nil,false}}
    if len(rule.ChatIDs)>0 {found:=false;for _,id:=range rule.ChatIDs {if id==in.ChatID {found=true;break}};if !found{return nil,false}}
    if len(rule.MediaTypes)>0 {found:=false;for _,kind:=range rule.MediaTypes {if kind==in.MediaType {found=true;break}};if !found{return nil,false}}
    containsFold:=func(values []string,value string)bool{if len(values)==0{return true};for _,v:=range values{if strings.EqualFold(v,value){return true}};return false}
    if !containsFold(rule.ChatTitles,in.ChatTitle)||!containsFold(rule.ChatUsernames,in.ChatUsername)||!containsFold(rule.SenderUsernames,in.SenderUsername)||!containsFold(rule.SenderNames,in.SenderName){return nil,false}
    if len(rule.SenderIDs)>0{found:=false;for _,id:=range rule.SenderIDs{if id==in.SenderID{found=true;break}};if !found{return nil,false}}
    allowed:=false;for _,kind:=range rule.ChatTypes {if kind==in.ChatType {allowed=true;break}}
    if !allowed {return nil,false}
    text:=in.Text;terms:=rule.Keywords;excluded:=rule.ExcludeTerms
    if !rule.CaseSensitive {text=strings.ToLower(text)}
    if textPresent {for _,term:=range excluded {needle:=term;if !rule.CaseSensitive {needle=strings.ToLower(needle)};if strings.Contains(text,needle){return nil,false}}}
    matched:=[]string{}
    if len(rule.Keywords)==0 && len(rule.KeywordGroups)==0 {
		// A media-only rule matches the selected media even when Telegram
		// includes a caption in the same update.
		if len(rule.MediaTypes)>0 {return []string{"media:"+in.MediaType},true}
		return nil,false
	}
    if !textPresent {return nil,false}
    if len(rule.KeywordGroups)>0 {
        matchedGroups:=make([][]string,0,len(rule.KeywordGroups))
        for _,group:=range rule.KeywordGroups {
            groupRule:=rule;groupRule.Keywords=group;groupRule.KeywordGroups=nil
            matched,ok:=keywordRuleMatches(groupRule,in)
            if ok {matchedGroups=append(matchedGroups,matched)} else {matchedGroups=append(matchedGroups,nil)}
        }
        if rule.GroupOperator=="all" {
            result:=[]string{}
            for _,group:=range matchedGroups {if len(group)==0{return nil,false};for _,term:=range group{result=append(result,"group:"+term)}}
            return result,true
        }
        result:=[]string{}
        for _,group:=range matchedGroups {for _,term:=range group{result=append(result,"group:"+term)}}
        return result,len(result)>0
    }
    switch rule.MatchMode {
    case "any":
        for _,term:=range terms {needle:=term;if !rule.CaseSensitive {needle=strings.ToLower(needle)};if strings.Contains(text,needle){matched=append(matched,term)}}
        return matched,len(matched)>0
    case "exact":
        for _,term:=range terms {needle:=term;if !rule.CaseSensitive {needle=strings.ToLower(needle)};if text==needle{matched=append(matched,term)}}
        return matched,len(matched)>0
    case "all":
        for _,term:=range terms {needle:=term;if !rule.CaseSensitive {needle=strings.ToLower(needle)};if !strings.Contains(text,needle){return nil,false};matched=append(matched,term)}
        return matched,true
    case "regex":
        for _,pattern:=range terms {compiledPattern:=pattern;if !rule.CaseSensitive {compiledPattern="(?i:"+pattern+")"};compiled,err:=regexp.Compile(compiledPattern);if err!=nil{return nil,false};if compiled.MatchString(in.Text){matched=append(matched,pattern)}}
        return matched,len(matched)>0
    default:return nil,false
    }
}

func (s *Service) CreateKeywordRule(ctx context.Context, tenantID string, rule KeywordRule)(KeywordRule,error) {
    rule,err:=normalizeKeywordRule(rule);if err!=nil{return rule,err}
    keywords,_:=json.Marshal(rule.Keywords);excluded,_:=json.Marshal(rule.ExcludeTerms);types,_:=json.Marshal(rule.ChatTypes);accounts,_:=json.Marshal(rule.AccountIDs);chats,_:=json.Marshal(rule.ChatIDs);media,_:=json.Marshal(rule.MediaTypes);chatTitles,_:=json.Marshal(rule.ChatTitles);chatUsernames,_:=json.Marshal(rule.ChatUsernames);senderIDs,_:=json.Marshal(rule.SenderIDs);senderUsernames,_:=json.Marshal(rule.SenderUsernames);senderNames,_:=json.Marshal(rule.SenderNames);keywordGroups,_:=json.Marshal(rule.KeywordGroups);replyTextByChatType,_:=json.Marshal(rule.ReplyTextByChatType)
    err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        // Serialize rule-count changes per tenant so concurrent requests cannot
        // exceed the 100 enabled-rule limit.
        var lockedTenant string
        if err:=tx.QueryRowContext(ctx,`SELECT id::text FROM tenants WHERE id=$1 FOR UPDATE`,tenantID).Scan(&lockedTenant);err!=nil{return err}
        var count int
        if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM tenant_keyword_rules WHERE tenant_id=$1 AND enabled=TRUE",tenantID).Scan(&count);err!=nil{return err}
        if rule.Enabled && count>=100{return errors.New("active keyword rule limit reached (100)")}
        for _,accountID:=range rule.AccountIDs {var exists bool;if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2)",tenantID,accountID).Scan(&exists);err!=nil{return err};if !exists{return errors.New("keyword filter account does not belong to tenant")}}
        if rule.ActionType=="media" { var exists bool; if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM tenant_media_assets WHERE tenant_id=$1 AND id=$2)",tenantID,rule.MediaAssetID).Scan(&exists);err!=nil{return err};if !exists{return ErrMediaAssetNotFound} }
        if rule.ActionType=="media_album" { for _,item:=range rule.MediaAlbumItems { var exists bool;if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM tenant_media_assets WHERE tenant_id=$1 AND id=$2)",tenantID,item.AssetID).Scan(&exists);err!=nil{return err};if !exists{return ErrMediaAssetNotFound} } }
        return tx.QueryRowContext(ctx,`INSERT INTO tenant_keyword_rules(tenant_id,name,match_mode,keywords,exclude_terms,chat_types,account_ids,chat_ids,media_types,chat_titles,chat_usernames,sender_ids,sender_usernames,sender_names,keyword_groups,group_operator,reply_text,reply_text_by_chat_type,action_type,contact_phone,contact_first_name,contact_last_name,media_asset_id,media_type,media_caption,media_album_items,case_sensitive,enabled)
            VALUES($1,$2,$3,$4::jsonb,$5::jsonb,$6::jsonb,$7::jsonb,$8::jsonb,$9::jsonb,$10::jsonb,$11::jsonb,$12::jsonb,$13::jsonb,$14::jsonb,$15::jsonb,$16,$17,$18::jsonb,$19,$20,$21,$22,$23,$24,$25,$26::jsonb,$27,$28)
            RETURNING id::text,created_at,updated_at`,tenantID,rule.Name,rule.MatchMode,string(keywords),string(excluded),string(types),string(accounts),string(chats),string(media),string(chatTitles),string(chatUsernames),string(senderIDs),string(senderUsernames),string(senderNames),string(keywordGroups),rule.GroupOperator,rule.ReplyText,string(replyTextByChatType),rule.ActionType,rule.ContactPhone,rule.ContactFirstName,rule.ContactLastName,keywordNullableMediaAssetID(rule.MediaAssetID),rule.MediaType,rule.MediaCaption,keywordAlbumJSON(rule.MediaAlbumItems),rule.CaseSensitive,rule.Enabled).Scan(&rule.ID,&rule.CreatedAt,&rule.UpdatedAt)
    })
    return rule,err
}

func (s *Service) UpdateKeywordRule(ctx context.Context,tenantID,id string,rule KeywordRule)(KeywordRule,error) {
    if !validUUID(id){return rule,errors.New("keyword rule not found")}
    rule,err:=normalizeKeywordRule(rule);if err!=nil{return rule,err}
    keywords,_:=json.Marshal(rule.Keywords);excluded,_:=json.Marshal(rule.ExcludeTerms);types,_:=json.Marshal(rule.ChatTypes);accounts,_:=json.Marshal(rule.AccountIDs);chats,_:=json.Marshal(rule.ChatIDs);media,_:=json.Marshal(rule.MediaTypes);chatTitles,_:=json.Marshal(rule.ChatTitles);chatUsernames,_:=json.Marshal(rule.ChatUsernames);senderIDs,_:=json.Marshal(rule.SenderIDs);senderUsernames,_:=json.Marshal(rule.SenderUsernames);senderNames,_:=json.Marshal(rule.SenderNames);keywordGroups,_:=json.Marshal(rule.KeywordGroups);replyTextByChatType,_:=json.Marshal(rule.ReplyTextByChatType)
    err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        // Use the same tenant row lock as create to serialize enable/disable
        // transitions against the active-rule cap.
        var lockedTenant string
        if err:=tx.QueryRowContext(ctx,`SELECT id::text FROM tenants WHERE id=$1 FOR UPDATE`,tenantID).Scan(&lockedTenant);err!=nil{return err}
        if rule.Enabled {var count int;if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM tenant_keyword_rules WHERE tenant_id=$1 AND enabled=TRUE AND id<>$2",tenantID,id).Scan(&count);err!=nil{return err};if count>=100{return errors.New("active keyword rule limit reached (100)")}}
        for _,accountID:=range rule.AccountIDs {var exists bool;if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2)",tenantID,accountID).Scan(&exists);err!=nil{return err};if !exists{return errors.New("keyword filter account does not belong to tenant")}}
        if rule.ActionType=="media" { var exists bool; if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM tenant_media_assets WHERE tenant_id=$1 AND id=$2)",tenantID,rule.MediaAssetID).Scan(&exists);err!=nil{return err};if !exists{return ErrMediaAssetNotFound} }
        if rule.ActionType=="media_album" { for _,item:=range rule.MediaAlbumItems { var exists bool;if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM tenant_media_assets WHERE tenant_id=$1 AND id=$2)",tenantID,item.AssetID).Scan(&exists);err!=nil{return err};if !exists{return ErrMediaAssetNotFound} } }
        result,err:=tx.ExecContext(ctx,`UPDATE tenant_keyword_rules SET name=$3,match_mode=$4,keywords=$5::jsonb,exclude_terms=$6::jsonb,chat_types=$7::jsonb,account_ids=$8::jsonb,chat_ids=$9::jsonb,media_types=$10::jsonb,chat_titles=$11::jsonb,chat_usernames=$12::jsonb,sender_ids=$13::jsonb,sender_usernames=$14::jsonb,sender_names=$15::jsonb,keyword_groups=$16::jsonb,group_operator=$17,reply_text=$18,reply_text_by_chat_type=$19::jsonb,action_type=$20,contact_phone=$21,contact_first_name=$22,contact_last_name=$23,media_asset_id=$24,media_type=$25,media_caption=$26,media_album_items=$27::jsonb,case_sensitive=$28,enabled=$29,updated_at=NOW()
            WHERE tenant_id=$1 AND id=$2`,tenantID,id,rule.Name,rule.MatchMode,string(keywords),string(excluded),string(types),string(accounts),string(chats),string(media),string(chatTitles),string(chatUsernames),string(senderIDs),string(senderUsernames),string(senderNames),string(keywordGroups),rule.GroupOperator,rule.ReplyText,string(replyTextByChatType),rule.ActionType,rule.ContactPhone,rule.ContactFirstName,rule.ContactLastName,keywordNullableMediaAssetID(rule.MediaAssetID),rule.MediaType,rule.MediaCaption,keywordAlbumJSON(rule.MediaAlbumItems),rule.CaseSensitive,rule.Enabled)
        if err!=nil{return err};n,_:=result.RowsAffected();if n==0{return errors.New("keyword rule not found")}
        rule.ID=id
        return tx.QueryRowContext(ctx,"SELECT created_at,updated_at FROM tenant_keyword_rules WHERE tenant_id=$1 AND id=$2",tenantID,id).Scan(&rule.CreatedAt,&rule.UpdatedAt)
    })
    return rule,err
}

func (s *Service) ListKeywordRules(ctx context.Context,tenantID string)([]KeywordRule,error) {
    out:=[]KeywordRule{}
    err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        rows,err:=tx.QueryContext(ctx,`SELECT id::text,name,match_mode,keywords,exclude_terms,chat_types,account_ids,chat_ids,media_types,chat_titles,chat_usernames,sender_ids,sender_usernames,sender_names,keyword_groups,group_operator,reply_text,reply_text_by_chat_type,action_type,contact_phone,contact_first_name,contact_last_name,COALESCE(media_asset_id::text,''),media_type,media_caption,media_album_items,case_sensitive,enabled,created_at,updated_at
            FROM tenant_keyword_rules WHERE tenant_id=$1 ORDER BY created_at DESC,id LIMIT 200`,tenantID);if err!=nil{return err};defer rows.Close()
        for rows.Next(){var r KeywordRule;var keywords,excluded,types,accounts,chats,media,chatTitles,chatUsernames,senderIDs,senderUsernames,senderNames,keywordGroups,replyTextByChatType,albumItems []byte;if err:=rows.Scan(&r.ID,&r.Name,&r.MatchMode,&keywords,&excluded,&types,&accounts,&chats,&media,&chatTitles,&chatUsernames,&senderIDs,&senderUsernames,&senderNames,&keywordGroups,&r.GroupOperator,&r.ReplyText,&replyTextByChatType,&r.ActionType,&r.ContactPhone,&r.ContactFirstName,&r.ContactLastName,&r.MediaAssetID,&r.MediaType,&r.MediaCaption,&albumItems,&r.CaseSensitive,&r.Enabled,&r.CreatedAt,&r.UpdatedAt);err!=nil{return err};if err:=json.Unmarshal(keywords,&r.Keywords);err!=nil{return err};if err:=json.Unmarshal(excluded,&r.ExcludeTerms);err!=nil{return err};if err:=json.Unmarshal(types,&r.ChatTypes);err!=nil{return err};if err:=json.Unmarshal(accounts,&r.AccountIDs);err!=nil{return err};if err:=json.Unmarshal(chats,&r.ChatIDs);err!=nil{return err};if err:=json.Unmarshal(media,&r.MediaTypes);err!=nil{return err};if err:=json.Unmarshal(chatTitles,&r.ChatTitles);err!=nil{return err};if err:=json.Unmarshal(chatUsernames,&r.ChatUsernames);err!=nil{return err};if err:=json.Unmarshal(senderIDs,&r.SenderIDs);err!=nil{return err};if err:=json.Unmarshal(senderUsernames,&r.SenderUsernames);err!=nil{return err};if err:=json.Unmarshal(senderNames,&r.SenderNames);err!=nil{return err};if err:=json.Unmarshal(keywordGroups,&r.KeywordGroups);err!=nil{return err};if err:=json.Unmarshal(replyTextByChatType,&r.ReplyTextByChatType);err!=nil{return err};if err:=json.Unmarshal(albumItems,&r.MediaAlbumItems);err!=nil{return err};out=append(out,r)}
        return rows.Err()
    })
    return out,err
}

func (s *Service) DeleteKeywordRule(ctx context.Context,tenantID,id string)error {
    if !validUUID(id){return errors.New("keyword rule not found")}
    return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{res,err:=tx.ExecContext(ctx,"DELETE FROM tenant_keyword_rules WHERE tenant_id=$1 AND id=$2",tenantID,id);if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("keyword rule not found")};return nil})
}

func (s *Service) ListKeywordMatches(ctx context.Context,tenantID,ruleID string,limit,offset int)([]KeywordMatch,int,error) {
    if ruleID!=""&&!validUUID(ruleID){return nil,0,ErrInvalidKeywordRule}
    if limit<1||limit>100{limit=50};if offset<0{offset=0}
    out:=[]KeywordMatch{};total:=0
    err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        where:="tenant_id=$1";args:=[]any{tenantID}
        if ruleID!=""{where+=" AND rule_id=$2";args=append(args,ruleID)}
        if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM tenant_keyword_matches WHERE "+where,args...).Scan(&total);err!=nil{return err}
        args=append(args,limit,offset);limitArg:=len(args)-1;offsetArg:=len(args)
        matchWhere:="m.tenant_id=$1"
        if ruleID!=""{matchWhere+=" AND m.rule_id=$2"}
        rows,err:=tx.QueryContext(ctx,`SELECT m.id,m.rule_id::text,r.name,m.telegram_account_id::text,m.chat_id,m.message_id,m.matched_terms,m.matched_at
            FROM tenant_keyword_matches m JOIN tenant_keyword_rules r ON r.tenant_id=m.tenant_id AND r.id=m.rule_id
            WHERE `+matchWhere+fmt.Sprintf(" ORDER BY m.matched_at DESC,m.id DESC LIMIT $%d OFFSET $%d",limitArg,offsetArg),args...)
        if err!=nil{return err};defer rows.Close()
        for rows.Next(){var m KeywordMatch;var terms []byte;if err:=rows.Scan(&m.ID,&m.RuleID,&m.RuleName,&m.AccountID,&m.ChatID,&m.MessageID,&terms,&m.MatchedAt);err!=nil{return err};if err:=json.Unmarshal(terms,&m.MatchedTerms);err!=nil{return err};out=append(out,m)}
        return rows.Err()
    })
    return out,total,err
}

func evaluateAndRecordKeywordMatches(ctx context.Context,tx *sql.Tx,tenantID string,in Incoming)error {
    if in.IsOutgoing || (strings.TrimSpace(in.Text)=="" && strings.TrimSpace(in.MediaType)=="") {return nil}
    rows,err:=tx.QueryContext(ctx,`SELECT id::text,name,match_mode,keywords,exclude_terms,chat_types,account_ids,chat_ids,media_types,chat_titles,chat_usernames,sender_ids,sender_usernames,sender_names,keyword_groups,group_operator,reply_text,reply_text_by_chat_type,action_type,contact_phone,contact_first_name,contact_last_name,COALESCE(media_asset_id::text,''),media_type,media_caption,media_album_items,case_sensitive,enabled
        FROM tenant_keyword_rules WHERE tenant_id=$1 AND enabled=TRUE ORDER BY created_at,id LIMIT 100`,tenantID);if err!=nil{return err}
    rules:=[]KeywordRule{}
    for rows.Next(){var r KeywordRule;var keywords,excluded,types,accounts,chats,media,chatTitles,chatUsernames,senderIDs,senderUsernames,senderNames,keywordGroups,replyTextByChatType,albumItems []byte;if err:=rows.Scan(&r.ID,&r.Name,&r.MatchMode,&keywords,&excluded,&types,&accounts,&chats,&media,&chatTitles,&chatUsernames,&senderIDs,&senderUsernames,&senderNames,&keywordGroups,&r.GroupOperator,&r.ReplyText,&replyTextByChatType,&r.ActionType,&r.ContactPhone,&r.ContactFirstName,&r.ContactLastName,&r.MediaAssetID,&r.MediaType,&r.MediaCaption,&albumItems,&r.CaseSensitive,&r.Enabled);err!=nil{rows.Close();return err};if err:=json.Unmarshal(keywords,&r.Keywords);err!=nil{rows.Close();return err};if err:=json.Unmarshal(excluded,&r.ExcludeTerms);err!=nil{rows.Close();return err};if err:=json.Unmarshal(types,&r.ChatTypes);err!=nil{rows.Close();return err};if err:=json.Unmarshal(accounts,&r.AccountIDs);err!=nil{rows.Close();return err};if err:=json.Unmarshal(chats,&r.ChatIDs);err!=nil{rows.Close();return err};if err:=json.Unmarshal(media,&r.MediaTypes);err!=nil{rows.Close();return err};if err:=json.Unmarshal(chatTitles,&r.ChatTitles);err!=nil{rows.Close();return err};if err:=json.Unmarshal(chatUsernames,&r.ChatUsernames);err!=nil{rows.Close();return err};if err:=json.Unmarshal(senderIDs,&r.SenderIDs);err!=nil{rows.Close();return err};if err:=json.Unmarshal(senderUsernames,&r.SenderUsernames);err!=nil{rows.Close();return err};if err:=json.Unmarshal(senderNames,&r.SenderNames);err!=nil{rows.Close();return err};if err:=json.Unmarshal(keywordGroups,&r.KeywordGroups);err!=nil{rows.Close();return err};if err:=json.Unmarshal(replyTextByChatType,&r.ReplyTextByChatType);err!=nil{rows.Close();return err};if err:=json.Unmarshal(albumItems,&r.MediaAlbumItems);err!=nil{rows.Close();return err};rules=append(rules,r)}
    if err:=rows.Err();err!=nil{rows.Close();return err};rows.Close()
    for _,rule:=range rules {matched,ok:=keywordRuleMatches(rule,in);if !ok{continue};raw,err:=json.Marshal(matched);if err!=nil{return err}
        var matchID int64
        err=tx.QueryRowContext(ctx,`INSERT INTO tenant_keyword_matches(tenant_id,rule_id,telegram_account_id,chat_id,message_id,matched_terms)
            VALUES($1,$2,$3,$4,$5,$6::jsonb) ON CONFLICT(rule_id,telegram_account_id,chat_id,message_id) DO NOTHING RETURNING id`,tenantID,rule.ID,in.AccountID,in.ChatID,in.MessageID,string(raw)).Scan(&matchID)
        if errors.Is(err,sql.ErrNoRows) {continue}
        if err!=nil{return err}
        actionText:=keywordReplyTextForChat(rule,in.ChatType)
        if rule.ActionType=="contact" && (in.ChatType=="private" || in.ChatType=="group" || in.ChatType=="supergroup") {
            _,err=tx.ExecContext(ctx,`INSERT INTO keyword_action_jobs(tenant_id,keyword_match_id,telegram_account_id,chat_id,reply_to_message_id,message_text,action_type,contact_phone,contact_first_name,contact_last_name)
                VALUES($1,$2,$3,$4,$5,'','contact',$6,$7,$8) ON CONFLICT(keyword_match_id) DO NOTHING`,tenantID,matchID,in.AccountID,in.ChatID,in.MessageID,rule.ContactPhone,rule.ContactFirstName,rule.ContactLastName)
            if err!=nil{return err}
        } else if rule.ActionType=="media" && (in.ChatType=="private" || in.ChatType=="group" || in.ChatType=="supergroup") {
            _,err=tx.ExecContext(ctx,`INSERT INTO keyword_action_jobs(tenant_id,keyword_match_id,telegram_account_id,chat_id,reply_to_message_id,message_text,action_type,media_asset_id,media_type,media_caption)
                VALUES($1,$2,$3,$4,$5,'','media',$6,$7,$8) ON CONFLICT(keyword_match_id) DO NOTHING`,tenantID,matchID,in.AccountID,in.ChatID,in.MessageID,rule.MediaAssetID,rule.MediaType,rule.MediaCaption)
            if err!=nil{return err}
        } else if rule.ActionType=="media_album" && (in.ChatType=="private" || in.ChatType=="group" || in.ChatType=="supergroup") {
            _,err=tx.ExecContext(ctx,`INSERT INTO keyword_action_jobs(tenant_id,keyword_match_id,telegram_account_id,chat_id,reply_to_message_id,message_text,action_type,media_album_items) VALUES($1,$2,$3,$4,$5,'','media_album',$6::jsonb) ON CONFLICT(keyword_match_id) DO NOTHING`,tenantID,matchID,in.AccountID,in.ChatID,in.MessageID,keywordAlbumJSON(rule.MediaAlbumItems));if err!=nil{return err}
        } else if actionText!="" {
            _,err=tx.ExecContext(ctx,`INSERT INTO keyword_action_jobs(tenant_id,keyword_match_id,telegram_account_id,chat_id,reply_to_message_id,message_text)
                VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(keyword_match_id) DO NOTHING`,tenantID,matchID,in.AccountID,in.ChatID,in.MessageID,actionText)
            if err!=nil{return err}
        }
    }
    return nil
}


type KeywordActionJob struct {
    ID string
    TenantID string
    AccountID string
    ChatID int64
    ReplyToMessageID int64
    Text string
    ActionType string
    ContactPhone string
    ContactFirstName string
    ContactLastName string
    MediaAssetID string
    MediaType string
    MediaCaption string
    MediaAlbumItems []KeywordMediaAlbumItem
    Attempts int
    LeaseUntil time.Time
}

func (s *Service) ClaimKeywordActionJobs(ctx context.Context, limit int) ([]KeywordActionJob,error) {
    if limit<1||limit>50 {limit=20}
    inactiveRows,err:=s.db.QueryContext(ctx,`SELECT id::text FROM tenants WHERE status<>'active' OR expires_at IS NULL OR expires_at<=NOW()`)
    if err!=nil{return nil,err}
    inactiveTenants:=[]string{}
    for inactiveRows.Next(){var id string;if err:=inactiveRows.Scan(&id);err!=nil{inactiveRows.Close();return nil,err};inactiveTenants=append(inactiveTenants,id)}
    if err:=inactiveRows.Err();err!=nil{inactiveRows.Close();return nil,err};inactiveRows.Close()
    for _,tenantID:=range inactiveTenants {
        if err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
            // A processing lease that is still live may represent an in-flight
            // Telegram RPC. Pause pending jobs and only reclaim expired leases.
            _,err:=tx.ExecContext(ctx,`UPDATE keyword_action_jobs SET status='paused',lease_until=NULL,last_error='tenant inactive; manual review required',updated_at=NOW()
                WHERE tenant_id=$1 AND (status='pending' OR (status='processing' AND lease_until<=NOW()))`,tenantID)
            return err
        });err!=nil{return nil,err}
    }
    rows,err:=s.db.QueryContext(ctx,`SELECT id::text FROM tenants WHERE status='active' AND expires_at>NOW() ORDER BY id`)
    if err!=nil{return nil,err}
    var tenants []string
    for rows.Next(){var id string;if err:=rows.Scan(&id);err!=nil{rows.Close();return nil,err};tenants=append(tenants,id)}
    if err:=rows.Err();err!=nil{rows.Close();return nil,err};rows.Close()
    jobs:=[]KeywordActionJob{}
    for _,tenantID:=range tenants {
        if len(jobs)>=limit {break}
        err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
            remaining:=limit-len(jobs);if remaining>5{remaining=5}
            picked,err:=tx.QueryContext(ctx,`WITH picked AS (
                SELECT j.id,j.tenant_id,j.telegram_account_id
                FROM keyword_action_jobs j JOIN telegram_accounts a ON a.tenant_id=j.tenant_id AND a.id=j.telegram_account_id
                WHERE j.tenant_id=$1
                AND EXISTS (SELECT 1 FROM tenants active_tenant WHERE active_tenant.id=j.tenant_id AND active_tenant.status='active' AND active_tenant.expires_at>NOW())
                AND ((j.status='pending' AND j.available_at<=NOW()) OR (j.status='processing' AND j.lease_until<NOW()))
                AND a.status='authorized' AND a.session_ciphertext IS NOT NULL
                AND COALESCE(a.telegram_send_next_at,'-infinity'::timestamptz)<=NOW()
                AND COALESCE(a.telegram_send_cooldown_until,'-infinity'::timestamptz)<=NOW()
                AND NOT EXISTS (SELECT 1 FROM message_reply_jobs x WHERE x.tenant_id=j.tenant_id AND x.telegram_account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
                AND NOT EXISTS (SELECT 1 FROM keyword_action_jobs x WHERE x.tenant_id=j.tenant_id AND x.telegram_account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
                AND NOT EXISTS (SELECT 1 FROM tenant_join_task_accounts x WHERE x.tenant_id=j.tenant_id AND x.telegram_account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
                AND NOT EXISTS (SELECT 1 FROM account_profile_update_jobs x WHERE x.tenant_id=j.tenant_id AND x.account_id=j.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
                AND NOT EXISTS (SELECT 1 FROM keyword_action_jobs e WHERE e.tenant_id=j.tenant_id AND e.telegram_account_id=j.telegram_account_id
                AND ((e.status='pending' AND e.available_at<=NOW()) OR (e.status='processing' AND e.lease_until<NOW())) AND (e.created_at,e.id)<(j.created_at,j.id))
                ORDER BY j.created_at,j.id FOR UPDATE OF j,a SKIP LOCKED LIMIT $2
            ), claimed AS (
                UPDATE keyword_action_jobs j SET status='processing',attempts=j.attempts+1,lease_until=NOW()+INTERVAL '3 minutes',updated_at=NOW()
                FROM picked p WHERE j.id=p.id AND j.tenant_id=p.tenant_id
                RETURNING j.id::text,j.tenant_id::text,j.telegram_account_id::text,j.chat_id,j.reply_to_message_id,j.message_text,j.action_type,j.contact_phone,j.contact_first_name,j.contact_last_name,COALESCE(j.media_asset_id::text,''),j.media_type,j.media_caption,j.media_album_items,j.attempts,j.lease_until
            ), scheduled AS (
                UPDATE telegram_accounts a SET telegram_send_next_at=NOW()+INTERVAL '2 seconds'
                FROM claimed c WHERE a.tenant_id=c.tenant_id::uuid AND a.id=c.telegram_account_id::uuid RETURNING a.id
            ) SELECT c.* FROM claimed c JOIN scheduled s ON s.id=c.telegram_account_id::uuid`,tenantID,remaining)
            if err!=nil{return err};defer picked.Close()
            for picked.Next(){var job KeywordActionJob;var albumItems []byte;if err:=picked.Scan(&job.ID,&job.TenantID,&job.AccountID,&job.ChatID,&job.ReplyToMessageID,&job.Text,&job.ActionType,&job.ContactPhone,&job.ContactFirstName,&job.ContactLastName,&job.MediaAssetID,&job.MediaType,&job.MediaCaption,&albumItems,&job.Attempts,&job.LeaseUntil);err!=nil{return err};if err:=json.Unmarshal(albumItems,&job.MediaAlbumItems);err!=nil{return err};jobs=append(jobs,job)}
            return picked.Err()
        });if err!=nil{return nil,err}
    }
    return jobs,nil
}

func keywordActionRetryDelay(attempt int,floodWait time.Duration) time.Duration {
    if floodWait > 0 { return floodWait + time.Second }
    delay:=time.Duration(attempt*30)*time.Second
    if delay<30*time.Second { delay=30*time.Second }
    if delay>15*time.Minute { delay=15*time.Minute }
    return delay
}

func (s *Service) FinishKeywordAction(ctx context.Context,job KeywordActionJob,telegramMessageID int64,sendErr error,floodWait time.Duration,messageIDLists ...[]int64) error {
    return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
        if sendErr==nil {
            if telegramMessageID<=0{return errors.New("invalid Telegram message id")}
            messageIDs:=[]int64{telegramMessageID}
            if len(messageIDLists)>0 && len(messageIDLists[0])>0 { messageIDs=messageIDLists[0] }
            seenMessageIDs:=make(map[int64]struct{},len(messageIDs))
            for _,id:=range messageIDs {
                if id<=0 { return errors.New("invalid Telegram message id in result") }
                if _,exists:=seenMessageIDs[id];exists { return errors.New("duplicate Telegram message id in result") }
                seenMessageIDs[id]=struct{}{}
            }
            if messageIDs[0]!=telegramMessageID { return errors.New("primary Telegram message id does not match result list") }
            encodedIDs,err:=json.Marshal(messageIDs);if err!=nil{return err}
            result,err:=tx.ExecContext(ctx,`UPDATE keyword_action_jobs SET status='sent',telegram_message_id=$3,telegram_message_ids=$4::jsonb,lease_until=NULL,last_error='',updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$5`,job.TenantID,job.ID,telegramMessageID,string(encodedIDs),job.LeaseUntil)
            if err!=nil{return err};n,_:=result.RowsAffected();if n==0{return errors.New("keyword action lease not found")};return nil
        }
        var tenantActive bool
        if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1 AND status='active' AND expires_at>NOW())`,job.TenantID).Scan(&tenantActive);err!=nil{return err}
        if !tenantActive {
            result,err:=tx.ExecContext(ctx,`UPDATE keyword_action_jobs SET status='paused',last_error='tenant inactive; manual review required',lease_until=NULL,updated_at=NOW()
                WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$3`,job.TenantID,job.ID,job.LeaseUntil)
            if err!=nil{return err};n,_:=result.RowsAffected();if n==0{return errors.New("keyword action lease not found")}
            if floodWait>0 {
                waitSeconds:=int64(floodWait/time.Second);if floodWait%time.Second!=0{waitSeconds++};waitSeconds++
                _,err=tx.ExecContext(ctx,`UPDATE telegram_accounts SET telegram_send_cooldown_until=GREATEST(COALESCE(telegram_send_cooldown_until,NOW()),NOW()+($1 * INTERVAL '1 second')) WHERE tenant_id=$2 AND id=$3`,waitSeconds,job.TenantID,job.AccountID)
            }
            return err
        }
        message:=sendErr.Error();if len(message)>500{message=message[:500]}
        delay:=keywordActionRetryDelay(job.Attempts,floodWait)
        status:="pending";if job.Attempts>=5&&floodWait<=0{status="failed"}
        result,err:=tx.ExecContext(ctx,`UPDATE keyword_action_jobs SET status=$3,last_error=$4,available_at=NOW()+($5 * INTERVAL '1 second'),lease_until=NULL,updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$6`,job.TenantID,job.ID,status,message,int64(delay.Seconds()),job.LeaseUntil)
        if err!=nil{return err};n,_:=result.RowsAffected();if n==0{return errors.New("keyword action lease not found")}
        if floodWait>0 { waitSeconds:=int64(floodWait/time.Second); if floodWait%time.Second!=0 {waitSeconds++}; waitSeconds++; if _,err:=tx.ExecContext(ctx,`UPDATE telegram_accounts SET telegram_send_cooldown_until=GREATEST(COALESCE(telegram_send_cooldown_until,NOW()),NOW()+($1 * INTERVAL '1 second')) WHERE tenant_id=$2 AND id=$3`,waitSeconds,job.TenantID,job.AccountID);err!=nil{return err} }
        return nil
    })
}

func (s *Service) ReleaseKeywordAction(ctx context.Context,job KeywordActionJob,delay time.Duration,reason string) error {
    if delay < time.Second { delay = time.Second }
    if delay > 15*time.Minute { delay = 15*time.Minute }
    reason = strings.TrimSpace(reason)
    if reason == "" { reason = "Telegram account temporarily unavailable" }
    if len(reason) > 500 { reason = reason[:500] }
    return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
        result,err:=tx.ExecContext(ctx,`UPDATE keyword_action_jobs
            SET status='pending',attempts=GREATEST(attempts-1,0),available_at=NOW()+($5 * INTERVAL '1 second'),
                lease_until=NULL,last_error=$3,updated_at=NOW()
            WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$4`,
            job.TenantID,job.ID,reason,job.LeaseUntil,int64(delay.Seconds()))
        if err!=nil{return err}
        n,_:=result.RowsAffected()
        if n==0{return errors.New("keyword action lease not found")}
        return nil
    })
}

func (s *Service) PauseKeywordAction(ctx context.Context,job KeywordActionJob,reason string) error {
    reason=strings.TrimSpace(reason);if reason==""{reason="tenant inactive; manual review required"};if len(reason)>500{reason=reason[:500]}
    return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
        result,err:=tx.ExecContext(ctx,`UPDATE keyword_action_jobs SET status='paused',lease_until=NULL,last_error=$3,updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until=$4`,job.TenantID,job.ID,reason,job.LeaseUntil)
        if err!=nil{return err};n,_:=result.RowsAffected();if n==0{return errors.New("keyword action lease not found")};return nil
    })
}


type KeywordActionRecord struct {
    ID string `json:"id"`
    MatchID int64 `json:"keyword_match_id"`
    RuleName string `json:"rule_name"`
    AccountID string `json:"account_id"`
    ChatID int64 `json:"chat_id"`
    ReplyToMessageID int64 `json:"reply_to_message_id"`
    Text string `json:"message_text"`
    ActionType string `json:"action_type"`
    ContactPhone string `json:"contact_phone,omitempty"`
    ContactFirstName string `json:"contact_first_name,omitempty"`
    ContactLastName string `json:"contact_last_name,omitempty"`
    MediaAssetID string `json:"media_asset_id,omitempty"`
    MediaType string `json:"media_type,omitempty"`
    MediaCaption string `json:"media_caption,omitempty"`
    MediaAlbumItems []KeywordMediaAlbumItem `json:"media_album_items,omitempty"`
    Status string `json:"status"`
    Attempts int `json:"attempts"`
    TelegramMessageID *int64 `json:"telegram_message_id,omitempty"`
    TelegramMessageIDs []int64 `json:"telegram_message_ids"`
    LastError string `json:"last_error"`
    AvailableAt time.Time `json:"available_at"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

func (s *Service) ListKeywordActions(ctx context.Context,tenantID,status string,limit,offset int)([]KeywordActionRecord,int,error) {
    if limit<1||limit>100{limit=50};if offset<0{offset=0}
    switch status {case "","pending","processing","sent","failed","paused":default:return nil,0,errors.New("invalid keyword action status")}
    items:=[]KeywordActionRecord{};total:=0
    err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        where:="j.tenant_id=$1";args:=[]any{tenantID}
        if status!=""{args=append(args,status);where+=" AND j.status=$2"}
        if err:=tx.QueryRowContext(ctx,`SELECT COUNT(*) FROM keyword_action_jobs j WHERE `+where,args...).Scan(&total);err!=nil{return err}
        args=append(args,limit,offset);li,oi:=len(args)-1,len(args)
        rows,err:=tx.QueryContext(ctx,`SELECT j.id::text,j.keyword_match_id,r.name,j.telegram_account_id::text,j.chat_id,j.reply_to_message_id,j.message_text,j.action_type,j.contact_phone,j.contact_first_name,j.contact_last_name,COALESCE(j.media_asset_id::text,''),j.media_type,j.media_caption,j.media_album_items,j.status,j.attempts,j.telegram_message_id,j.telegram_message_ids,j.last_error,j.available_at,j.created_at,j.updated_at
            FROM keyword_action_jobs j JOIN tenant_keyword_matches m ON m.id=j.keyword_match_id AND m.tenant_id=j.tenant_id
            JOIN tenant_keyword_rules r ON r.id=m.rule_id AND r.tenant_id=m.tenant_id
            WHERE `+where+fmt.Sprintf(" ORDER BY j.created_at DESC,j.id DESC LIMIT $%d OFFSET $%d",li,oi),args...)
        if err!=nil{return err};defer rows.Close()
        for rows.Next(){var item KeywordActionRecord;var albumItems,messageIDs []byte;if err:=rows.Scan(&item.ID,&item.MatchID,&item.RuleName,&item.AccountID,&item.ChatID,&item.ReplyToMessageID,&item.Text,&item.ActionType,&item.ContactPhone,&item.ContactFirstName,&item.ContactLastName,&item.MediaAssetID,&item.MediaType,&item.MediaCaption,&albumItems,&item.Status,&item.Attempts,&item.TelegramMessageID,&messageIDs,&item.LastError,&item.AvailableAt,&item.CreatedAt,&item.UpdatedAt);err!=nil{return err};if err:=json.Unmarshal(albumItems,&item.MediaAlbumItems);err!=nil{return err};if err:=json.Unmarshal(messageIDs,&item.TelegramMessageIDs);err!=nil{return err};items=append(items,item)}
        return rows.Err()
    })
    return items,total,err
}

func (s *Service) ChangeKeywordActionStatus(ctx context.Context,tenantID,id,action string) error {
    if !validUUID(id){return errors.New("keyword action not found")}
    var query string
    switch action {
    case "pause":
        query=`UPDATE keyword_action_jobs SET status='paused',lease_until=NULL,last_error='paused by user',updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status IN ('pending','failed')`
    case "resume":
        query=`UPDATE keyword_action_jobs j SET status='pending',attempts=0,available_at=NOW(),lease_until=NULL,last_error='',updated_at=NOW() FROM tenants t WHERE j.tenant_id=$1 AND j.id=$2 AND j.status='paused' AND t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW()`
    case "retry":
        query=`UPDATE keyword_action_jobs j SET status='pending',attempts=0,available_at=NOW(),lease_until=NULL,last_error='',updated_at=NOW() FROM tenants t WHERE j.tenant_id=$1 AND j.id=$2 AND j.status='failed' AND t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW()`
    default:return errors.New("unsupported keyword action")
    }
    return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        result,err:=tx.ExecContext(ctx,query,tenantID,id);if err!=nil{return err}
        n,_:=result.RowsAffected();if n==0{return errors.New("keyword action not found or state does not allow action")}
        return nil
    })
}


func keywordReplyTextForChat(rule KeywordRule,chatType string) string {
    key:=""
    switch chatType {
    case "private": key="private"
    case "group","supergroup": key="group"
    case "channel": return ""
    default: return rule.ReplyText
    }
    if specific:=rule.ReplyTextByChatType[key];specific!="" {return specific}
    return rule.ReplyText
}


func keywordAlbumJSON(items []KeywordMediaAlbumItem) string { if len(items)==0{return "[]"};b,err:=json.Marshal(items);if err!=nil{return "[]"};return string(b) }

func keywordNullableMediaAssetID(value string) any { if value == "" { return nil }; return value }
