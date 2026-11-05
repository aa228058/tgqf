package accounts

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "strings"
 "time"
 "os"
 "net"
 "strconv"
 device "github.com/mtgo-labs/device-manager"
)
type Service struct{db *sql.DB}
type Account struct{ID string `json:"id"`;Phone string `json:"phone"`;DisplayName string `json:"display_name"`;GroupName string `json:"group_name"`;Tags []string `json:"tags"`;Status string `json:"status"`;HasSession bool `json:"has_session"`;HasProxy bool `json:"has_proxy"`;LastError string `json:"last_error"`;JoinFloodWaitUntil *time.Time `json:"join_flood_wait_until,omitempty"`;SendCooldownUntil *time.Time `json:"send_cooldown_until,omitempty"`;CreatedAt time.Time `json:"created_at"`;UpdatedAt time.Time `json:"updated_at"`}
func New(db *sql.DB)*Service{return &Service{db:db}}
func(s *Service)Create(ctx context.Context,tenantID,phone,name,group,session string,proxy json.RawMessage)(Account,error){
 phone=strings.TrimSpace(phone);name=strings.TrimSpace(name);group=strings.TrimSpace(group);session=strings.TrimSpace(session)
 if tenantID==""||len(phone)<5||len(phone)>32||session==""||len(session)>65536||len(name)>120||len(group)>80{return Account{},errors.New("invalid account fields")}
 if len(proxy)>16384{return Account{},errors.New("proxy configuration is too large")}
 var proxyCipher []byte;var err error
 if len(proxy)>0&&string(proxy)!="null"&&string(proxy)!="{}"{var v map[string]any;if err=json.Unmarshal(proxy,&v);err!=nil{return Account{},errors.New("proxy must be a JSON object")};proxyCipher,err=encrypt(proxy);if err!=nil{return Account{},err}}
 sessionCipher,err:=encrypt([]byte(session));if err!=nil{return Account{},err}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return Account{},err};defer tx.Rollback()
 if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,tenantID);err!=nil{return Account{},err}
 var limit int
 if err=tx.QueryRowContext(ctx,`SELECT account_limit FROM tenants WHERE id=$1 AND status='active' AND expires_at>NOW() FOR UPDATE`,tenantID).Scan(&limit);err!=nil{return Account{},errors.New("tenant subscription inactive")}
 var count int
 if err=tx.QueryRowContext(ctx,`SELECT COUNT(*) FROM telegram_accounts WHERE tenant_id=$1`,tenantID).Scan(&count);err!=nil{return Account{},err}
 if count>=limit{return Account{},fmt.Errorf("account limit reached (%d)",limit)}
 var a Account
 err=tx.QueryRowContext(ctx,`INSERT INTO telegram_accounts(tenant_id,phone,display_name,group_name,status,session_ciphertext,session_key_version,proxy_ciphertext,proxy_id) VALUES($1,$2,$3,$4,'pending',$5,1,$6,CASE WHEN $6::bytea IS NULL THEN (SELECT id FROM tenant_proxies WHERE tenant_id=$1 AND is_default=TRUE LIMIT 1) ELSE NULL END) RETURNING id::text,phone,display_name,group_name,tags,status,session_ciphertext IS NOT NULL,(proxy_ciphertext IS NOT NULL OR proxy_id IS NOT NULL),last_error,created_at,updated_at`,tenantID,phone,name,group,sessionCipher,nullBytes(proxyCipher)).Scan(&a.ID,&a.Phone,&a.DisplayName,&a.GroupName,&a.Tags,&a.Status,&a.HasSession,&a.HasProxy,&a.LastError,&a.CreatedAt,&a.UpdatedAt)
 if err!=nil{return Account{},err};if err=insertDeviceProfile(ctx,tx,tenantID,a.ID);err!=nil{return Account{},err};if err=tx.Commit();err!=nil{return Account{},err};return a,nil
}
func nullBytes(v []byte)any{if len(v)==0{return nil};return v}
func(s *Service)List(ctx context.Context,tenantID string)([]Account,error){
 out:=[]Account{}
 err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{rows,err:=tx.QueryContext(ctx,`SELECT id::text,COALESCE(phone,''),display_name,group_name,tags,status,session_ciphertext IS NOT NULL,(proxy_ciphertext IS NOT NULL OR proxy_id IS NOT NULL),last_error,created_at,updated_at FROM telegram_accounts WHERE tenant_id=$1 ORDER BY created_at DESC`,tenantID);if err!=nil{return err};defer rows.Close();for rows.Next(){var a Account;if err:=rows.Scan(&a.ID,&a.Phone,&a.DisplayName,&a.GroupName,&a.Tags,&a.Status,&a.HasSession,&a.HasProxy,&a.LastError,&a.JoinFloodWaitUntil,&a.SendCooldownUntil,&a.CreatedAt,&a.UpdatedAt);err!=nil{return err};out=append(out,a)};return rows.Err()})
 return out,err
}

type AccountListOptions struct {
	Query string
	GroupName string
	Status string
	Limit int
	Offset int
}


func normalizeAccountListOptions(opts AccountListOptions)(AccountListOptions,error){
	opts.Query=strings.TrimSpace(opts.Query);opts.GroupName=strings.TrimSpace(opts.GroupName);opts.Status=strings.ToLower(strings.TrimSpace(opts.Status))
	if len(opts.Query)>120||len(opts.GroupName)>80{return opts,errors.New("invalid account filter")}
	switch opts.Status {case "","pending","authorized","error","disconnected":default:return opts,errors.New("invalid account status filter")}
	if opts.Limit<1||opts.Limit>100{opts.Limit=50};if opts.Offset<0{opts.Offset=0}
	return opts,nil
}

func (s *Service) ListPage(ctx context.Context,tenantID string,opts AccountListOptions)([]Account,int,error){
	var err error
	opts,err=normalizeAccountListOptions(opts);if err!=nil{return nil,0,err}
	out:=[]Account{};total:=0
	err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		filter:="tenant_id=$1 AND ($2='' OR phone ILIKE '%'||$2||'%' OR display_name ILIKE '%'||$2||'%') AND ($3='' OR group_name=$3) AND ($4='' OR status=$4)"
		if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM telegram_accounts WHERE "+filter,tenantID,opts.Query,opts.GroupName,opts.Status).Scan(&total);err!=nil{return err}
		rows,err:=tx.QueryContext(ctx,"SELECT id::text,COALESCE(phone,''),display_name,group_name,tags,status,session_ciphertext IS NOT NULL,(proxy_ciphertext IS NOT NULL OR proxy_id IS NOT NULL),last_error,join_flood_wait_until,telegram_send_cooldown_until,created_at,updated_at FROM telegram_accounts WHERE "+filter+" ORDER BY created_at DESC,id DESC LIMIT $5 OFFSET $6",tenantID,opts.Query,opts.GroupName,opts.Status,opts.Limit,opts.Offset)
		if err!=nil{return err};defer rows.Close()
		for rows.Next(){var a Account;if err:=rows.Scan(&a.ID,&a.Phone,&a.DisplayName,&a.GroupName,&a.Tags,&a.Status,&a.HasSession,&a.HasProxy,&a.LastError,&a.CreatedAt,&a.UpdatedAt);err!=nil{return err};out=append(out,a)}
		return rows.Err()
	})
	return out,total,err
}

func (s *Service) DeleteBatch(ctx context.Context,tenantID string,accountIDs []string)([]string,error) {
	ids,err:=normalizeBulkDeleteAccountIDs(accountIDs)
	if err!=nil{return nil,err}
	deleted:=make([]string,0,len(ids))
	err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		rows,err:=tx.QueryContext(ctx,`SELECT id::text FROM telegram_accounts WHERE tenant_id=$1 AND id=ANY($2::uuid[]) FOR UPDATE`,tenantID,ids)
		if err!=nil{return err}
		found:=make(map[string]bool,len(ids))
		for rows.Next(){var id string;if err:=rows.Scan(&id);err!=nil{rows.Close();return err};found[id]=true}
		if err:=rows.Err();err!=nil{rows.Close();return err};rows.Close()
		if len(found)!=len(ids){return errors.New("one or more accounts were not found in this tenant; no accounts were deleted")}
		res,err:=tx.ExecContext(ctx,`DELETE FROM telegram_accounts WHERE tenant_id=$1 AND id=ANY($2::uuid[])`,tenantID,ids)
		if err!=nil{return err}
		n,err:=res.RowsAffected();if err!=nil{return err}
		if int(n)!=len(ids){return errors.New("account delete count changed during transaction")}
		deleted=append(deleted,ids...)
		return nil
	})
	if err!=nil{return nil,err}
	return deleted,nil
}

func normalizeAccountDisplayName(value string)(string,error){
 value=strings.TrimSpace(value)
 if len([]rune(value))>120{return "",errors.New("display name must be at most 120 characters")}
 for _,r:=range value{if r<32||r==127{return "",errors.New("display name cannot contain control characters")}}
 return value,nil
}

func(s *Service)UpdateDisplayName(ctx context.Context,tenantID,accountID,displayName string)error{
 normalized,err:=normalizeAccountDisplayName(displayName)
 if err!=nil{return err}
 return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
  res,err:=tx.ExecContext(ctx,`UPDATE telegram_accounts SET display_name=$3,updated_at=NOW() WHERE tenant_id=$1 AND id=$2`,tenantID,accountID,normalized)
  if err!=nil{return err}
  n,err:=res.RowsAffected();if err!=nil{return err}
  if n==0{return errors.New("account not found")}
  return nil
 })
}

func(s *Service)Delete(ctx context.Context,tenantID,id string)error{return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{res,err:=tx.ExecContext(ctx,`DELETE FROM telegram_accounts WHERE tenant_id=$1 AND id=$2`,tenantID,id);if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("account not found")};return nil})}
func(s *Service)SetGroup(ctx context.Context,tenantID,id,group string)error{group=strings.TrimSpace(group);if len(group)>80{return errors.New("group name must be at most 80 characters")};return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{res,err:=tx.ExecContext(ctx,`UPDATE telegram_accounts SET group_name=$3,updated_at=NOW() WHERE tenant_id=$1 AND id=$2`,tenantID,id,group);if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("account not found")};return nil})}

func normalizeAccountTags(input []string) ([]string,error) {
	if len(input)>20 { return nil,errors.New("an account may have at most 20 tags") }
	out:=make([]string,0,len(input));seen:=map[string]bool{}
	for _,tag:=range input {
		tag=strings.TrimSpace(tag)
		if tag=="" { return nil,errors.New("account tags cannot be empty") }
		if len([]rune(tag))>32 { return nil,errors.New("account tags must be at most 32 characters") }
		for _,r:=range tag { if r<32 || r==127 { return nil,errors.New("account tags cannot contain control characters") } }
		key:=strings.ToLower(tag);if seen[key] { continue };seen[key]=true;out=append(out,tag)
	}
	return out,nil
}

// SetAccountTags replaces one account's tags. An empty slice clears all tags.
func (s *Service) SetAccountTags(ctx context.Context,tenantID,accountID string,tags []string) error {
	normalized,err:=normalizeAccountTags(tags);if err!=nil{return err}
	return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		res,err:=tx.ExecContext(ctx,"UPDATE telegram_accounts SET tags=$3::text[],updated_at=NOW() WHERE tenant_id=$1 AND id=$2",tenantID,accountID,normalized)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("account not found")};return nil
	})
}

// SetAccountTagsBatch atomically replaces tags for every selected account.
func (s *Service) SetAccountTagsBatch(ctx context.Context,tenantID string,accountIDs,tags []string)(int,error) {
	ids,_,err:=normalizeBulkGroupInput(accountIDs,"");if err!=nil{return 0,err}
	normalized,err:=normalizeAccountTags(tags);if err!=nil{return 0,err}
	updated:=0
	err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		for _,id:=range ids {
			res,err:=tx.ExecContext(ctx,"UPDATE telegram_accounts SET tags=$3::text[],updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid",tenantID,id,normalized)
			if err!=nil{return err};n,err:=res.RowsAffected();if err!=nil{return err};if n!=1{return errors.New("one or more accounts were not found in this tenant")};updated+=int(n)
		}
		return nil
	})
	if err!=nil{return 0,err};return updated,nil
}
func withTenantTx(ctx context.Context,db *sql.DB,tenantID string,fn func(*sql.Tx)error)error{if tenantID==""{return errors.New("tenant scope is required")};tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback();if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,tenantID);err!=nil{return err};if err=fn(tx);err!=nil{return err};return tx.Commit()}


func insertDeviceProfileIfIncomplete(ctx context.Context,tx *sql.Tx,tenantID,accountID string)error{
	profile:=device.IOS.Generate(accountID)
	profile.LangCode=envValue("DEFAULT_LANG_CODE",profile.LangCode)
	profile.SystemLangCode=envValue("DEFAULT_SYSTEM_LANG_CODE",profile.SystemLangCode)
	profile.LangPack=envValue("DEFAULT_LANG_PACK",profile.LangPack)
	profile.AppVersion=envValue("TELEGRAM_APP_VERSION",profile.AppVersion)
	if profile.DeviceModel==""||profile.SystemVersion==""||profile.AppVersion==""{return errors.New("device-manager returned an incomplete iOS profile")}
	_,err:=tx.ExecContext(ctx,`INSERT INTO telegram_device_profiles(tenant_id,telegram_account_id,platform,device_model,system_version,app_version,lang_code,system_lang_code,lang_pack,generation_seed,package_id)
		VALUES($1,$2,'ios',$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT(telegram_account_id) DO UPDATE SET
		 device_model=CASE WHEN telegram_device_profiles.device_model='' THEN EXCLUDED.device_model ELSE telegram_device_profiles.device_model END,
		 system_version=CASE WHEN telegram_device_profiles.system_version='' THEN EXCLUDED.system_version ELSE telegram_device_profiles.system_version END,
		 app_version=CASE WHEN telegram_device_profiles.app_version='' THEN EXCLUDED.app_version ELSE telegram_device_profiles.app_version END,
		 lang_code=CASE WHEN telegram_device_profiles.lang_code='' THEN EXCLUDED.lang_code ELSE telegram_device_profiles.lang_code END,
		 system_lang_code=CASE WHEN telegram_device_profiles.system_lang_code='' THEN EXCLUDED.system_lang_code ELSE telegram_device_profiles.system_lang_code END,
		 lang_pack=CASE WHEN telegram_device_profiles.lang_pack='' THEN EXCLUDED.lang_pack ELSE telegram_device_profiles.lang_pack END,
		 package_id=CASE WHEN telegram_device_profiles.package_id='' THEN EXCLUDED.package_id ELSE telegram_device_profiles.package_id END,
		 updated_at=NOW()
		 WHERE telegram_device_profiles.tenant_id=EXCLUDED.tenant_id
		 AND (telegram_device_profiles.device_model='' OR telegram_device_profiles.system_version='' OR telegram_device_profiles.app_version='')`,
		tenantID,accountID,profile.DeviceModel,profile.SystemVersion,profile.AppVersion,profile.LangCode,profile.SystemLangCode,profile.LangPack,accountID,profile.PackageID)
	return err
}

func insertDeviceProfile(ctx context.Context,tx *sql.Tx,tenantID,accountID string)error{
 profile:=device.IOS.Generate(accountID)
 profile.LangCode=envValue("DEFAULT_LANG_CODE",profile.LangCode)
 profile.SystemLangCode=envValue("DEFAULT_SYSTEM_LANG_CODE",profile.SystemLangCode)
 profile.LangPack=envValue("DEFAULT_LANG_PACK",profile.LangPack)
 profile.AppVersion=envValue("TELEGRAM_APP_VERSION",profile.AppVersion)
 if profile.DeviceModel==""||profile.SystemVersion==""||profile.AppVersion==""{return errors.New("device-manager returned an incomplete iOS profile")}
 _,err:=tx.ExecContext(ctx,`INSERT INTO telegram_device_profiles(tenant_id,telegram_account_id,platform,device_model,system_version,app_version,lang_code,system_lang_code,lang_pack,generation_seed,package_id) VALUES($1,$2,'ios',$3,$4,$5,$6,$7,$8,$9,$10)`,tenantID,accountID,profile.DeviceModel,profile.SystemVersion,profile.AppVersion,profile.LangCode,profile.SystemLangCode,profile.LangPack,accountID,profile.PackageID)
 return err
}
func envValue(key,fallback string)string{if v:=strings.TrimSpace(os.Getenv(key));v!=""{return v};return fallback}


// RuntimeAccount contains decrypted credentials only for the isolated worker.
// Never serialize this value or expose it from an HTTP handler.
type RuntimeAccount struct {
 TenantID string
 ID string
 Session string
 ProxyJSON string
 DeviceProfile device.Profile
 ReconnectAttempts int
 ReconnectAt time.Time
}

// LoadAuthorizedRuntimeAccounts loads active tenants first, then reads each
// tenant's accounts under its own RLS-scoped transaction.
func (s *Service) LoadAuthorizedRuntimeAccounts(ctx context.Context) ([]RuntimeAccount, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id::text FROM tenants WHERE status='active' AND expires_at>NOW()`)
	if err != nil { return nil, err }
	var tenantIDs []string
	for rows.Next() { var id string; if err := rows.Scan(&id); err != nil { rows.Close(); return nil, err }; tenantIDs = append(tenantIDs, id) }
	if err := rows.Err(); err != nil { rows.Close(); return nil, err }
	rows.Close()

	var out []RuntimeAccount
	for _, tenantID := range tenantIDs {
		err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		// Backfill legacy accounts from the same stable device-manager generator.
		// Only incomplete/missing rows are repaired; complete saved identities stay unchanged.
		missingRows, err := tx.QueryContext(ctx, `SELECT a.id::text
			FROM telegram_accounts a
			LEFT JOIN telegram_device_profiles dp ON dp.tenant_id=a.tenant_id AND dp.telegram_account_id=a.id
			WHERE a.tenant_id=$1 AND a.status IN ('pending','authorized','error')
			AND a.session_ciphertext IS NOT NULL
			AND (dp.telegram_account_id IS NULL OR dp.device_model='' OR dp.system_version='' OR dp.app_version='')`, tenantID)
		if err != nil { return err }
		var repairIDs []string
		for missingRows.Next() { var id string; if err:=missingRows.Scan(&id);err!=nil{missingRows.Close();return err};repairIDs=append(repairIDs,id) }
		if err:=missingRows.Err();err!=nil{missingRows.Close();return err};missingRows.Close()
		for _, accountID := range repairIDs {
			if err:=insertDeviceProfileIfIncomplete(ctx,tx,tenantID,accountID);err!=nil{return fmt.Errorf("repair device profile for account %s: %w",accountID,err)}
		}
		accountRows, err := tx.QueryContext(ctx, `SELECT a.id::text,a.session_ciphertext,a.proxy_ciphertext,
				p.protocol,p.host,p.port,p.username,p.password_ciphertext,
                COALESCE(dp.device_model,''),COALESCE(dp.system_version,''),COALESCE(dp.app_version,''),
                COALESCE(dp.lang_code,''),COALESCE(dp.system_lang_code,''),COALESCE(dp.lang_pack,''),COALESCE(dp.package_id,''),
				COALESCE(rt.reconnect_attempts,0),rt.reconnect_at
				FROM telegram_accounts a
				LEFT JOIN tenant_proxies p ON p.tenant_id=a.tenant_id AND p.id=a.proxy_id
                LEFT JOIN telegram_device_profiles dp ON dp.tenant_id=a.tenant_id AND dp.telegram_account_id=a.id
				LEFT JOIN telegram_account_runtime rt ON rt.tenant_id=a.tenant_id AND rt.account_id=a.id
				WHERE a.tenant_id=$1 AND a.status IN ('pending','authorized','error') AND a.session_ciphertext IS NOT NULL`, tenantID)
			if err != nil { return err }
			defer accountRows.Close()
			for accountRows.Next() {
				var item RuntimeAccount
				var sessionCipher, proxyCipher, proxyPasswordCipher []byte
				var protocol, host, username sql.NullString
                var deviceModel, systemVersion, appVersion, langCode, systemLangCode, langPack, packageID sql.NullString
				var port sql.NullInt64
				var reconnectAt sql.NullTime
				item.TenantID = tenantID
				if err := accountRows.Scan(&item.ID,&sessionCipher,&proxyCipher,&protocol,&host,&port,&username,&proxyPasswordCipher,
                    &deviceModel,&systemVersion,&appVersion,&langCode,&systemLangCode,&langPack,&packageID,
                    &item.ReconnectAttempts,&reconnectAt); err != nil { return err }
                if !deviceModel.Valid || !systemVersion.Valid || !appVersion.Valid {
                    return fmt.Errorf("persisted device profile missing or incomplete for account %s", item.ID)
                }
                item.DeviceProfile = device.Profile{DeviceModel:deviceModel.String,SystemVersion:systemVersion.String,AppVersion:appVersion.String,
                    LangCode:langCode.String,SystemLangCode:systemLangCode.String,LangPack:langPack.String,PackageID:packageID.String}
				plain, err := decrypt(sessionCipher); if err != nil { return fmt.Errorf("decrypt session for account %s: %w", item.ID, err) }
				item.Session = string(plain)
				if reconnectAt.Valid { item.ReconnectAt = reconnectAt.Time }
				if protocol.Valid {
					password := ""
					if len(proxyPasswordCipher)>0 { secret,err:=decrypt(proxyPasswordCipher);if err!=nil{return fmt.Errorf("decrypt proxy password for account %s: %w",item.ID,err)};password=string(secret) }
					config,err:=json.Marshal(map[string]any{"protocol":protocol.String,"addr":net.JoinHostPort(host.String,strconv.FormatInt(port.Int64,10)),"username":username.String,"password":password})
					if err!=nil{return err};item.ProxyJSON=string(config)
				} else if len(proxyCipher)>0 {
					proxy,err:=decrypt(proxyCipher);if err!=nil{return fmt.Errorf("decrypt proxy for account %s: %w",item.ID,err)};item.ProxyJSON=string(proxy)
				}
				out = append(out, item)
			}
			return accountRows.Err()
		})
		if err != nil { return nil, err }
	}
	return out, nil
}

// LoadDeviceProfile reads the account's persisted device identity under tenant scope.
func (s *Service) LoadDeviceProfile(ctx context.Context, tenantID, accountID string) (device.Profile, error) {
    var profile device.Profile
    err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
        err := tx.QueryRowContext(ctx, `SELECT device_model,system_version,app_version,lang_code,system_lang_code,lang_pack,package_id
            FROM telegram_device_profiles WHERE tenant_id=$1 AND telegram_account_id=$2`, tenantID, accountID).
            Scan(&profile.DeviceModel,&profile.SystemVersion,&profile.AppVersion,&profile.LangCode,&profile.SystemLangCode,&profile.LangPack,&profile.PackageID)
        if errors.Is(err, sql.ErrNoRows) { return fmt.Errorf("persisted device profile not found for account %s", accountID) }
        if err != nil { return err }
        if profile.DeviceModel=="" || profile.SystemVersion=="" || profile.AppVersion=="" {
            return fmt.Errorf("persisted device profile is incomplete for account %s", accountID)
        }
        return nil
    })
    return profile, err
}



// SaveRuntimeSession encrypts a refreshed session before updating its row.
func (s *Service) SaveRuntimeSession(ctx context.Context, tenantID, accountID, session string) error {
	if strings.TrimSpace(session) == "" { return errors.New("session is required") }
	ciphertext, err := encrypt([]byte(session)); if err != nil { return err }
	return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE telegram_accounts SET session_ciphertext=$3,session_key_version=1,updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='authorized'`, tenantID, accountID, ciphertext)
		if err != nil { return err }; n, _ := result.RowsAffected(); if n == 0 { return errors.New("authorized account not found") }; return nil
	})
}

// SetRuntimeStatus updates operational state without exposing credentials.
func (s *Service) SetRuntimeStatus(ctx context.Context, tenantID, accountID, status, lastError string) error {
	switch status { case "authorized", "disconnected", "error": default: return errors.New("invalid account runtime status") }
	if len(lastError) > 500 { lastError = lastError[:500] }
	return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE telegram_accounts SET status=$3,last_error=$4,updated_at=NOW() WHERE tenant_id=$1 AND id=$2`, tenantID, accountID, status, lastError)
		if err != nil { return err }; n, _ := result.RowsAffected(); if n == 0 { return errors.New("account not found") }; return nil
	})
}

func (s *Service) CreatePending(ctx context.Context, tenantID, group string, proxy json.RawMessage) (Account, error) {
 group=strings.TrimSpace(group)
 if tenantID=="" || len(group)>80 { return Account{},errors.New("invalid pending account fields") }
 if len(proxy)>16384 { return Account{},errors.New("proxy configuration is too large") }
 var proxyCipher []byte
 if len(proxy)>0 && string(proxy)!="null" && string(proxy)!="{}" {
  var value map[string]any
  if err:=json.Unmarshal(proxy,&value);err!=nil || value==nil { return Account{},errors.New("proxy must be a JSON object") }
  var err error; proxyCipher,err=encrypt(proxy);if err!=nil{return Account{},err}
 }
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return Account{},err};defer tx.Rollback()
 if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,tenantID);err!=nil{return Account{},err}
 var limit int
 if err=tx.QueryRowContext(ctx,`SELECT account_limit FROM tenants WHERE id=$1 AND status='active' AND expires_at>NOW() FOR UPDATE`,tenantID).Scan(&limit);err!=nil{return Account{},errors.New("tenant subscription inactive")}
 var count int
 if err=tx.QueryRowContext(ctx,`SELECT COUNT(*) FROM telegram_accounts WHERE tenant_id=$1`,tenantID).Scan(&count);err!=nil{return Account{},err}
 if count>=limit{return Account{},fmt.Errorf("account limit reached (%d)",limit)}
 var a Account
 err=tx.QueryRowContext(ctx,`INSERT INTO telegram_accounts(tenant_id,phone,display_name,group_name,status,proxy_ciphertext,proxy_id) VALUES($1,NULL,'',$2,'pending',$3,CASE WHEN $3::bytea IS NULL THEN (SELECT id FROM tenant_proxies WHERE tenant_id=$1 AND is_default=TRUE LIMIT 1) ELSE NULL END) RETURNING id::text,'',display_name,group_name,tags,status,false,(proxy_ciphertext IS NOT NULL OR proxy_id IS NOT NULL),last_error,created_at,updated_at`,tenantID,group,nullBytes(proxyCipher)).Scan(&a.ID,&a.Phone,&a.DisplayName,&a.GroupName,&a.Tags,&a.Status,&a.HasSession,&a.HasProxy,&a.LastError,&a.CreatedAt,&a.UpdatedAt)
 if err!=nil{return Account{},err}
 if err=insertDeviceProfile(ctx,tx,tenantID,a.ID);err!=nil{return Account{},err}
 if err=tx.Commit();err!=nil{return Account{},err}
 return a,nil
}


func (s *Service) LoadPendingProxyJSON(ctx context.Context,tenantID,accountID string)(string,error){
	var result string
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		var proxyID sql.NullString
		var proxyCipher []byte
		if err:=tx.QueryRowContext(ctx,`SELECT proxy_id::text,proxy_ciphertext FROM telegram_accounts WHERE tenant_id=$1 AND id=$2 AND status='pending'`,tenantID,accountID).Scan(&proxyID,&proxyCipher);err!=nil{return err}
		if proxyID.Valid && proxyID.String!="" { var err error;result,err=s.LoadProxyJSON(ctx,tx,tenantID,proxyID.String);return err }
		if len(proxyCipher)>0 { plain,err:=decrypt(proxyCipher);if err!=nil{return err};result=string(plain) }
		return nil
	})
	return result,err
}

func (s *Service) IsPending(ctx context.Context, tenantID, accountID string) (bool, error) {
	var pending bool
	err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `SELECT EXISTS(
			SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2 AND status='pending'
		)`, tenantID, accountID).Scan(&pending)
	})
	return pending, err
}

func (s *Service) CompleteQRAuthorization(ctx context.Context, tenantID, accountID, phone, displayName, session string) (Account,error) {
 phone=strings.TrimSpace(phone);displayName=strings.TrimSpace(displayName);session=strings.TrimSpace(session)
 if tenantID==""||len(phone)<5||len(phone)>32||session==""||len(session)>65536||len(displayName)>120{return Account{},errors.New("invalid Telegram authorization result")}
 ciphertext,err:=encrypt([]byte(session));if err!=nil{return Account{},err}
 var a Account
 err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
  return tx.QueryRowContext(ctx,`UPDATE telegram_accounts SET phone=$3,display_name=$4,status='authorized',session_ciphertext=$5,session_key_version=1,last_error='',updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='pending' RETURNING id::text,COALESCE(phone,''),display_name,group_name,tags,status,true,(proxy_ciphertext IS NOT NULL OR proxy_id IS NOT NULL),last_error,created_at,updated_at`,tenantID,accountID,phone,displayName,ciphertext).Scan(&a.ID,&a.Phone,&a.DisplayName,&a.GroupName,&a.Tags,&a.Status,&a.HasSession,&a.HasProxy,&a.LastError,&a.CreatedAt,&a.UpdatedAt)
 })
 if err!=nil{return Account{},err};return a,nil
}


// SaveRuntimeReconnectState persists retry timing so worker restarts do not
// reset exponential backoff and reconnect every account at once.
func (s *Service) SaveRuntimeReconnectState(ctx context.Context, tenantID, accountID string, attempts int, nextTry time.Time) error {
 if attempts < 1 { attempts = 1 }
 if attempts > 30 { attempts = 30 }
 return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
  _, err := tx.ExecContext(ctx, `INSERT INTO telegram_account_runtime(tenant_id,account_id,reconnect_attempts,reconnect_at,updated_at)
   VALUES($1,$2,$3,$4,NOW())
   ON CONFLICT(tenant_id,account_id) DO UPDATE SET reconnect_attempts=EXCLUDED.reconnect_attempts,reconnect_at=EXCLUDED.reconnect_at,updated_at=NOW()`, tenantID,accountID,attempts,nextTry)
  return err
 })
}

// ClearRuntimeReconnectState resets the retry counter after a successful connection.
func (s *Service) ClearRuntimeReconnectState(ctx context.Context, tenantID, accountID string) error {
 return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
  _, err := tx.ExecContext(ctx, `DELETE FROM telegram_account_runtime WHERE tenant_id=$1 AND account_id=$2`,tenantID,accountID)
  return err
 })
}

func normalizeBulkGroupInput(ids []string, group string) ([]string, string, error) {
	group = strings.TrimSpace(group)
	if len(group) > 80 { return nil, "", errors.New("group name must be at most 80 characters") }
	if len(ids) == 0 || len(ids) > 100 { return nil, "", errors.New("select between 1 and 100 accounts") }
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' { return nil, "", errors.New("invalid account id") }
		for i, ch := range id { if i==8 || i==13 || i==18 || i==23 { continue }; if !((ch>='0'&&ch<='9')||(ch>='a'&&ch<='f')) { return nil, "", errors.New("invalid account id") } }
		if seen[id] { return nil, "", errors.New("duplicate account id") }
		seen[id] = true
		out = append(out, id)
	}
	return out, group, nil
}

// SetGroupBatch updates group labels atomically for accounts in the current tenant.
func (s *Service) SetGroupBatch(ctx context.Context, tenantID string, ids []string, group string) (int, error) {
	ids, group, err := normalizeBulkGroupInput(ids, group)
	if err != nil { return 0, err }
	updated := 0
	err = withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		for _, id := range ids {
			result, err := tx.ExecContext(ctx, `UPDATE telegram_accounts SET group_name=$3,updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid`, tenantID, id, group)
			if err != nil { return err }
			n, err := result.RowsAffected()
			if err != nil { return err }
			if n != 1 { return errors.New("one or more accounts were not found in this tenant") }
			updated += int(n)
		}
		return nil
	})
	if err != nil { return 0, err }
	return updated, nil
}


// ApplyBulkRuntimeAction changes the persisted desired runtime state for a
// tenant's accounts. The worker reconciler performs the actual MTGo stop/start.
func normalizeBulkRuntimeActionInput(accountIDs []string, action string) ([]string, string, error) {
	ids, _, err := normalizeBulkGroupInput(accountIDs, "")
	if err != nil { return nil, "", err }
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "disconnect" && action != "reconnect" { return nil, "", errors.New("action must be disconnect or reconnect") }
	return ids, action, nil
}

func (s *Service) ApplyBulkRuntimeAction(ctx context.Context, tenantID string, accountIDs []string, action string) (int, error) {
	ids, action, err := normalizeBulkRuntimeActionInput(accountIDs, action)
	if err != nil { return 0, err }
	changed := 0
	err = withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		for _, id := range ids {
			var status string
			var hasSession bool
			err := tx.QueryRowContext(ctx, `SELECT status,session_ciphertext IS NOT NULL FROM telegram_accounts WHERE tenant_id=$1 AND id=$2::uuid FOR UPDATE`, tenantID, id).Scan(&status, &hasSession)
			if errors.Is(err, sql.ErrNoRows) { return errors.New("one or more accounts were not found in this tenant") }
			if err != nil { return err }
			target := "disconnected"
			if action == "reconnect" { target = "authorized" }
			if status == target { if action == "reconnect" && !hasSession { return errors.New("account has no saved Telegram session") }; continue }
			if action == "disconnect" {
				if status != "authorized" && status != "disconnected" { return fmt.Errorf("account %s cannot be disconnected from status %s", id, status) }
			} else {
				if status != "disconnected" && status != "authorized" { return fmt.Errorf("account %s cannot be reconnected from status %s", id, status) }
				if !hasSession { return errors.New("account has no saved Telegram session") }
			}
			res, err := tx.ExecContext(ctx, `UPDATE telegram_accounts SET status=$3,last_error='',updated_at=NOW() WHERE tenant_id=$1 AND id=$2::uuid`, tenantID, id, target)
			if err != nil { return err }
			n, err := res.RowsAffected(); if err != nil { return err }; changed += int(n)
		}
		return nil
	})
	if err != nil { return 0, err }
	return changed, nil
}

// OwnsAccount checks tenant ownership without returning account secrets.
func (s *Service) OwnsAccount(ctx context.Context, tenantID, accountID string) (bool, error) {
 owned:=false
 err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx) error {
  return tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2::uuid)`,tenantID,accountID).Scan(&owned)
 })
 return owned,err
}
