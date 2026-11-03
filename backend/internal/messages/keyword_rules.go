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

type KeywordRule struct {
    ID string `json:"id"`
    Name string `json:"name"`
    MatchMode string `json:"match_mode"`
    Keywords []string `json:"keywords"`
    ExcludeTerms []string `json:"exclude_terms"`
    ChatTypes []string `json:"chat_types"`
    AccountIDs []string `json:"account_ids"`
    ChatIDs []int64 `json:"chat_ids"`
    MediaTypes []string `json:"media_types"`
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
    if len(rule.Keywords)==0 || len(rule.Keywords)>20 || len(rule.ExcludeTerms)>20 { return rule, fmt.Errorf("%w: provide 1-20 keywords and at most 20 exclude terms",ErrInvalidKeywordRule) }
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
    rule.ExcludeTerms,err=normalizeTerms(rule.ExcludeTerms,false);if err!=nil{return rule,err}
    if rule.ChatTypes==nil {rule.ChatTypes=[]string{"private","group","channel"}}
    if rule.AccountIDs==nil {rule.AccountIDs=[]string{}}
    if rule.ChatIDs==nil {rule.ChatIDs=[]int64{}}
    if rule.MediaTypes==nil {rule.MediaTypes=[]string{}}
    if len(rule.AccountIDs)>100 || len(rule.ChatIDs)>1000 || len(rule.MediaTypes)>8 {return rule,fmt.Errorf("%w: keyword filter limits exceeded",ErrInvalidKeywordRule)}
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
    return rule,nil
}

func keywordRuleMatches(rule KeywordRule, in Incoming) ([]string,bool) {
    if in.IsOutgoing || strings.TrimSpace(in.Text)=="" {return nil,false}
    if len(rule.AccountIDs)>0 {found:=false;for _,id:=range rule.AccountIDs {if id==in.AccountID {found=true;break}};if !found{return nil,false}}
    if len(rule.ChatIDs)>0 {found:=false;for _,id:=range rule.ChatIDs {if id==in.ChatID {found=true;break}};if !found{return nil,false}}
    if len(rule.MediaTypes)>0 {found:=false;for _,kind:=range rule.MediaTypes {if kind==in.MediaType {found=true;break}};if !found{return nil,false}}
    allowed:=false;for _,kind:=range rule.ChatTypes {if kind==in.ChatType {allowed=true;break}}
    if !allowed {return nil,false}
    text:=in.Text;terms:=rule.Keywords;excluded:=rule.ExcludeTerms
    if !rule.CaseSensitive {text=strings.ToLower(text)}
    for _,term:=range excluded {needle:=term;if !rule.CaseSensitive {needle=strings.ToLower(needle)};if strings.Contains(text,needle){return nil,false}}
    matched:=[]string{}
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
    keywords,_:=json.Marshal(rule.Keywords);excluded,_:=json.Marshal(rule.ExcludeTerms);types,_:=json.Marshal(rule.ChatTypes);accounts,_:=json.Marshal(rule.AccountIDs);chats,_:=json.Marshal(rule.ChatIDs);media,_:=json.Marshal(rule.MediaTypes)
    err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        var count int
        if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM tenant_keyword_rules WHERE tenant_id=$1 AND enabled=TRUE",tenantID).Scan(&count);err!=nil{return err}
        if rule.Enabled && count>=100{return errors.New("active keyword rule limit reached (100)")}
        for _,accountID:=range rule.AccountIDs {var exists bool;if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2)",tenantID,accountID).Scan(&exists);err!=nil{return err};if !exists{return errors.New("keyword filter account does not belong to tenant")}}
        return tx.QueryRowContext(ctx,`INSERT INTO tenant_keyword_rules(tenant_id,name,match_mode,keywords,exclude_terms,chat_types,account_ids,chat_ids,media_types,case_sensitive,enabled)
            VALUES($1,$2,$3,$4::jsonb,$5::jsonb,$6::jsonb,$7::jsonb,$8::jsonb,$9::jsonb,$10,$11)
            RETURNING id::text,created_at,updated_at`,tenantID,rule.Name,rule.MatchMode,string(keywords),string(excluded),string(types),rule.CaseSensitive,rule.Enabled).Scan(&rule.ID,&rule.CreatedAt,&rule.UpdatedAt)
    })
    return rule,err
}

func (s *Service) UpdateKeywordRule(ctx context.Context,tenantID,id string,rule KeywordRule)(KeywordRule,error) {
    if !validUUID(id){return rule,errors.New("keyword rule not found")}
    rule,err:=normalizeKeywordRule(rule);if err!=nil{return rule,err}
    keywords,_:=json.Marshal(rule.Keywords);excluded,_:=json.Marshal(rule.ExcludeTerms);types,_:=json.Marshal(rule.ChatTypes);accounts,_:=json.Marshal(rule.AccountIDs);chats,_:=json.Marshal(rule.ChatIDs);media,_:=json.Marshal(rule.MediaTypes)
    err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        if rule.Enabled {var count int;if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM tenant_keyword_rules WHERE tenant_id=$1 AND enabled=TRUE AND id<>$2",tenantID,id).Scan(&count);err!=nil{return err};if count>=100{return errors.New("active keyword rule limit reached (100)")}}
        for _,accountID:=range rule.AccountIDs {var exists bool;if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2)",tenantID,accountID).Scan(&exists);err!=nil{return err};if !exists{return errors.New("keyword filter account does not belong to tenant")}}
        result,err:=tx.ExecContext(ctx,`UPDATE tenant_keyword_rules SET name=$3,match_mode=$4,keywords=$5::jsonb,exclude_terms=$6::jsonb,chat_types=$7::jsonb,account_ids=$8::jsonb,chat_ids=$9::jsonb,media_types=$10::jsonb,case_sensitive=$11,enabled=$12,updated_at=NOW()
            WHERE tenant_id=$1 AND id=$2`,tenantID,id,rule.Name,rule.MatchMode,string(keywords),string(excluded),string(types),string(accounts),string(chats),string(media),rule.CaseSensitive,rule.Enabled)
        if err!=nil{return err};n,_:=result.RowsAffected();if n==0{return errors.New("keyword rule not found")}
        rule.ID=id
        return tx.QueryRowContext(ctx,"SELECT created_at,updated_at FROM tenant_keyword_rules WHERE tenant_id=$1 AND id=$2",tenantID,id).Scan(&rule.CreatedAt,&rule.UpdatedAt)
    })
    return rule,err
}

func (s *Service) ListKeywordRules(ctx context.Context,tenantID string)([]KeywordRule,error) {
    out:=[]KeywordRule{}
    err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        rows,err:=tx.QueryContext(ctx,`SELECT id::text,name,match_mode,keywords,exclude_terms,chat_types,account_ids,chat_ids,media_types,case_sensitive,enabled,created_at,updated_at
            FROM tenant_keyword_rules WHERE tenant_id=$1 ORDER BY created_at DESC,id LIMIT 200`,tenantID);if err!=nil{return err};defer rows.Close()
        for rows.Next(){var r KeywordRule;var keywords,excluded,types,accounts,chats,media []byte;if err:=rows.Scan(&r.ID,&r.Name,&r.MatchMode,&keywords,&excluded,&types,&accounts,&chats,&media,&r.CaseSensitive,&r.Enabled,&r.CreatedAt,&r.UpdatedAt);err!=nil{return err};if err:=json.Unmarshal(keywords,&r.Keywords);err!=nil{return err};if err:=json.Unmarshal(excluded,&r.ExcludeTerms);err!=nil{return err};if err:=json.Unmarshal(types,&r.ChatTypes);err!=nil{return err};if err:=json.Unmarshal(accounts,&r.AccountIDs);err!=nil{return err};if err:=json.Unmarshal(chats,&r.ChatIDs);err!=nil{return err};if err:=json.Unmarshal(media,&r.MediaTypes);err!=nil{return err};out=append(out,r)}
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
    rows,err:=tx.QueryContext(ctx,`SELECT id::text,name,match_mode,keywords,exclude_terms,chat_types,account_ids,chat_ids,media_types,case_sensitive,enabled
        FROM tenant_keyword_rules WHERE tenant_id=$1 AND enabled=TRUE ORDER BY created_at,id LIMIT 100`,tenantID);if err!=nil{return err}
    rules:=[]KeywordRule{}
    for rows.Next(){var r KeywordRule;var keywords,excluded,types,accounts,chats,media []byte;if err:=rows.Scan(&r.ID,&r.Name,&r.MatchMode,&keywords,&excluded,&types,&accounts,&chats,&media,&r.CaseSensitive,&r.Enabled);err!=nil{rows.Close();return err};if err:=json.Unmarshal(keywords,&r.Keywords);err!=nil{rows.Close();return err};if err:=json.Unmarshal(excluded,&r.ExcludeTerms);err!=nil{rows.Close();return err};if err:=json.Unmarshal(types,&r.ChatTypes);err!=nil{rows.Close();return err};if err:=json.Unmarshal(accounts,&r.AccountIDs);err!=nil{rows.Close();return err};if err:=json.Unmarshal(chats,&r.ChatIDs);err!=nil{rows.Close();return err};if err:=json.Unmarshal(media,&r.MediaTypes);err!=nil{rows.Close();return err};rules=append(rules,r)}
    if err:=rows.Err();err!=nil{rows.Close();return err};rows.Close()
    for _,rule:=range rules {matched,ok:=keywordRuleMatches(rule,in);if !ok{continue};raw,err:=json.Marshal(matched);if err!=nil{return err}
        if _,err=tx.ExecContext(ctx,`INSERT INTO tenant_keyword_matches(tenant_id,rule_id,telegram_account_id,chat_id,message_id,matched_terms)
            VALUES($1,$2,$3,$4,$5,$6::jsonb) ON CONFLICT(rule_id,telegram_account_id,chat_id,message_id) DO NOTHING`,tenantID,rule.ID,in.AccountID,in.ChatID,in.MessageID,string(raw));err!=nil{return err}
    }
    return nil
}
