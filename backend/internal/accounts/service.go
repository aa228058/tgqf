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
type Account struct{ID string `json:"id"`;Phone string `json:"phone"`;DisplayName string `json:"display_name"`;GroupName string `json:"group_name"`;Status string `json:"status"`;HasSession bool `json:"has_session"`;HasProxy bool `json:"has_proxy"`;LastError string `json:"last_error"`;CreatedAt time.Time `json:"created_at"`;UpdatedAt time.Time `json:"updated_at"`}
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
 err=tx.QueryRowContext(ctx,`INSERT INTO telegram_accounts(tenant_id,phone,display_name,group_name,status,session_ciphertext,session_key_version,proxy_ciphertext) VALUES($1,$2,$3,$4,'pending',$5,1,$6) RETURNING id::text,phone,display_name,group_name,status,session_ciphertext IS NOT NULL,(proxy_ciphertext IS NOT NULL OR proxy_id IS NOT NULL),last_error,created_at,updated_at`,tenantID,phone,name,group,sessionCipher,nullBytes(proxyCipher)).Scan(&a.ID,&a.Phone,&a.DisplayName,&a.GroupName,&a.Status,&a.HasSession,&a.HasProxy,&a.LastError,&a.CreatedAt,&a.UpdatedAt)
 if err!=nil{return Account{},err};if err=insertDeviceProfile(ctx,tx,tenantID,a.ID);err!=nil{return Account{},err};if err=tx.Commit();err!=nil{return Account{},err};return a,nil
}
func nullBytes(v []byte)any{if len(v)==0{return nil};return v}
func(s *Service)List(ctx context.Context,tenantID string)([]Account,error){
 out:=[]Account{}
 err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{rows,err:=tx.QueryContext(ctx,`SELECT id::text,COALESCE(phone,''),display_name,group_name,status,session_ciphertext IS NOT NULL,(proxy_ciphertext IS NOT NULL OR proxy_id IS NOT NULL),last_error,created_at,updated_at FROM telegram_accounts WHERE tenant_id=$1 ORDER BY created_at DESC`,tenantID);if err!=nil{return err};defer rows.Close();for rows.Next(){var a Account;if err:=rows.Scan(&a.ID,&a.Phone,&a.DisplayName,&a.GroupName,&a.Status,&a.HasSession,&a.HasProxy,&a.LastError,&a.CreatedAt,&a.UpdatedAt);err!=nil{return err};out=append(out,a)};return rows.Err()})
 return out,err
}
func(s *Service)Delete(ctx context.Context,tenantID,id string)error{return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{res,err:=tx.ExecContext(ctx,`DELETE FROM telegram_accounts WHERE tenant_id=$1 AND id=$2`,tenantID,id);if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("account not found")};return nil})}
func(s *Service)SetGroup(ctx context.Context,tenantID,id,group string)error{group=strings.TrimSpace(group);if len(group)>80{return errors.New("group name must be at most 80 characters")};return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{res,err:=tx.ExecContext(ctx,`UPDATE telegram_accounts SET group_name=$3,updated_at=NOW() WHERE tenant_id=$1 AND id=$2`,tenantID,id,group);if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("account not found")};return nil})}
func withTenantTx(ctx context.Context,db *sql.DB,tenantID string,fn func(*sql.Tx)error)error{if tenantID==""{return errors.New("tenant scope is required")};tx,err:=db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback();if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,tenantID);err!=nil{return err};if err=fn(tx);err!=nil{return err};return tx.Commit()}

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
func storedDeviceProfile(accountID, deviceModel, systemVersion, appVersion, langCode, systemLangCode, langPack, packageID string) (device.Profile, error) {
	profile := device.IOS.Generate(accountID)
	if deviceModel != "" { profile.DeviceModel = deviceModel }
	if systemVersion != "" { profile.SystemVersion = systemVersion }
	if appVersion != "" { profile.AppVersion = appVersion }
	if langCode != "" { profile.LangCode = langCode }
	if systemLangCode != "" { profile.SystemLangCode = systemLangCode }
	if langPack != "" { profile.LangPack = langPack }
	if packageID != "" { profile.PackageID = packageID }
	if profile.DeviceModel == "" || profile.SystemVersion == "" || profile.AppVersion == "" {
		return device.Profile{}, errors.New("stored device profile is incomplete")
	}
	return profile, nil
}

// GetDeviceProfile returns the persisted device identity for one account.
func (s *Service) GetDeviceProfile(ctx context.Context, tenantID, accountID string) (device.Profile, error) {
	var profile device.Profile
	err := withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		var deviceModel, systemVersion, appVersion, langCode, systemLangCode, langPack, packageID string
		if err := tx.QueryRowContext(ctx, `SELECT device_model,system_version,app_version,lang_code,system_lang_code,lang_pack,package_id
			FROM telegram_device_profiles WHERE tenant_id=$1 AND telegram_account_id=$2`, tenantID, accountID).
			Scan(&deviceModel,&systemVersion,&appVersion,&langCode,&systemLangCode,&langPack,&packageID); err != nil { return err }
		var err error
		profile, err = storedDeviceProfile(accountID,deviceModel,systemVersion,appVersion,langCode,systemLangCode,langPack,packageID)
		return err
	})
	return profile, err
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
			accountRows, err := tx.QueryContext(ctx, `SELECT a.id::text,a.session_ciphertext,a.proxy_ciphertext,
				p.protocol,p.host,p.port,p.username,p.password_ciphertext,
				COALESCE(rt.reconnect_attempts,0),rt.reconnect_at,
				COALESCE(dp.device_model,''),COALESCE(dp.system_version,''),COALESCE(dp.app_version,''),
				COALESCE(dp.lang_code,''),COALESCE(dp.system_lang_code,''),COALESCE(dp.lang_pack,''),COALESCE(dp.package_id,'')
				FROM telegram_accounts a
				LEFT JOIN tenant_proxies p ON p.tenant_id=a.tenant_id AND p.id=a.proxy_id
				LEFT JOIN telegram_account_runtime rt ON rt.tenant_id=a.tenant_id AND rt.account_id=a.id
				LEFT JOIN telegram_device_profiles dp ON dp.tenant_id=a.tenant_id AND dp.telegram_account_id=a.id
				WHERE a.tenant_id=$1 AND a.status IN ('authorized','error') AND a.session_ciphertext IS NOT NULL`, tenantID)
			if err != nil { return err }
			defer accountRows.Close()
			for accountRows.Next() {
				var item RuntimeAccount
				var sessionCipher, proxyCipher, proxyPasswordCipher []byte
				var protocol, host, username sql.NullString
				var port sql.NullInt64
				var reconnectAt sql.NullTime
				item.TenantID = tenantID
				var deviceModel, systemVersion, appVersion, langCode, systemLangCode, langPack, packageID string
				if err := accountRows.Scan(&item.ID,&sessionCipher,&proxyCipher,&protocol,&host,&port,&username,&proxyPasswordCipher,&item.ReconnectAttempts,&reconnectAt,&deviceModel,&systemVersion,&appVersion,&langCode,&systemLangCode,&langPack,&packageID); err != nil { return err }
				item.DeviceProfile, err = storedDeviceProfile(item.ID, deviceModel, systemVersion, appVersion, langCode, systemLangCode, langPack, packageID)
				if err != nil { return fmt.Errorf("load device profile for account %s: %w", item.ID, err) }
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
 err=tx.QueryRowContext(ctx,`INSERT INTO telegram_accounts(tenant_id,phone,display_name,group_name,status,proxy_ciphertext) VALUES($1,NULL,'',$2,'pending',$3) RETURNING id::text,'',display_name,group_name,status,false,(proxy_ciphertext IS NOT NULL OR proxy_id IS NOT NULL),last_error,created_at,updated_at`,tenantID,group,nullBytes(proxyCipher)).Scan(&a.ID,&a.Phone,&a.DisplayName,&a.GroupName,&a.Status,&a.HasSession,&a.HasProxy,&a.LastError,&a.CreatedAt,&a.UpdatedAt)
 if err!=nil{return Account{},err}
 if err=insertDeviceProfile(ctx,tx,tenantID,a.ID);err!=nil{return Account{},err}
 if err=tx.Commit();err!=nil{return Account{},err}
 return a,nil
}

func (s *Service) CompleteQRAuthorization(ctx context.Context, tenantID, accountID, phone, displayName, session string) (Account,error) {
 phone=strings.TrimSpace(phone);displayName=strings.TrimSpace(displayName);session=strings.TrimSpace(session)
 if tenantID==""||len(phone)<5||len(phone)>32||session==""||len(session)>65536||len(displayName)>120{return Account{},errors.New("invalid Telegram authorization result")}
 ciphertext,err:=encrypt([]byte(session));if err!=nil{return Account{},err}
 var a Account
 err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
  return tx.QueryRowContext(ctx,`UPDATE telegram_accounts SET phone=$3,display_name=$4,status='authorized',session_ciphertext=$5,session_key_version=1,last_error='',updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='pending' RETURNING id::text,COALESCE(phone,''),display_name,group_name,status,true,(proxy_ciphertext IS NOT NULL OR proxy_id IS NOT NULL),last_error,created_at,updated_at`,tenantID,accountID,phone,displayName,ciphertext).Scan(&a.ID,&a.Phone,&a.DisplayName,&a.GroupName,&a.Status,&a.HasSession,&a.HasProxy,&a.LastError,&a.CreatedAt,&a.UpdatedAt)
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
