package auth

import (
 "context"
 "crypto/rand"
 "crypto/sha256"
 "database/sql"
 "encoding/base64"
 "encoding/json"
 "errors"
 "strings"
 "time"

 "github.com/jackc/pgx/v5/pgconn"
 "github.com/redis/go-redis/v9"
 "golang.org/x/crypto/bcrypt"
)
var(ErrInvalidCredentials=errors.New("invalid username or password");ErrAccountInactive=errors.New("account is disabled or subscription is inactive");ErrCardInvalid=errors.New("activation card is invalid, used, or disabled");ErrUsernameTaken=errors.New("username already exists"))
type Service struct{db *sql.DB; redis *redis.Client};type User struct{ID string `json:"id"`;TenantID string `json:"tenant_id,omitempty"`;Username string `json:"username"`;Role string `json:"role"`};type Session struct{Token string;ExpiresAt time.Time;User User}
func New(db *sql.DB, cache ...*redis.Client)*Service{service:=&Service{db:db};if len(cache)>0{service.redis=cache[0]};return service}
func(s *Service)Register(ctx context.Context,username,password,tenantName string)(User,error){if !s.SystemConfigBool(ctx,"registration_enabled",true){return User{},errors.New("new tenant registration is disabled by system administrator")};username=strings.TrimSpace(username);tenantName=strings.TrimSpace(tenantName);if len(username)<3||len(username)>64||len(password)<10||len(password)>72||tenantName==""||len(tenantName)>120{return User{},errors.New("username must be 3-64 characters, password 10-72 bytes, tenant name 1-120 characters")};hash,err:=bcrypt.GenerateFromPassword([]byte(password),bcrypt.DefaultCost);if err!=nil{return User{},err};tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return User{},err};defer tx.Rollback();var tid,uid string;if err=tx.QueryRowContext(ctx,`INSERT INTO tenants(name,status,account_limit) VALUES($1,'active',$2) RETURNING id::text`,tenantName,s.SystemConfigInt(ctx,"default_account_limit",5)).Scan(&tid);err!=nil{return User{},err};if err=tx.QueryRowContext(ctx,`INSERT INTO users(tenant_id,username,password_hash,role) VALUES($1,$2,$3,'tenant_admin') RETURNING id::text`,tid,username,string(hash)).Scan(&uid);err!=nil{var pe *pgconn.PgError;if errors.As(err,&pe)&&pe.Code=="23505"{return User{},ErrUsernameTaken};return User{},err};if err=tx.Commit();err!=nil{return User{},err};return User{ID:uid,TenantID:tid,Username:username,Role:"tenant_admin"},nil}
func(s *Service)Login(ctx context.Context,username,password,label string)(Session,error){return s.LoginAs(ctx,username,password,label,"")}
func(s *Service)LoginAs(ctx context.Context,username,password,label,loginType string)(Session,error){
 var u User;var hash,status string
 err:=s.db.QueryRowContext(ctx,"SELECT id::text,COALESCE(tenant_id::text,''),username,role,password_hash,status FROM users WHERE username=$1",strings.TrimSpace(username)).Scan(&u.ID,&u.TenantID,&u.Username,&u.Role,&hash,&status)
 if err!=nil||bcrypt.CompareHashAndPassword([]byte(hash),[]byte(password))!=nil{return Session{},ErrInvalidCredentials}
 if status!="active"{return Session{},ErrAccountInactive}
 if loginType!="" {
  if loginType!="admin"&&loginType!="tenant"{return Session{},errors.New("invalid login type")}
  if loginType=="admin"&&u.Role!="super_admin"{return Session{},errors.New("selected management login requires a system administrator account")}
  if loginType=="tenant"&&(u.Role!="tenant_admin"||u.TenantID==""){return Session{},errors.New("selected tenant login requires an active tenant administrator account")}
 }
 token,err:=randomToken();if err!=nil{return Session{},err}
 ttl:=s.SystemConfigInt(ctx,"auth_session_ttl_hours",720);expires:=time.Now().Add(time.Duration(ttl)*time.Hour)
 _,err=s.db.ExecContext(ctx,"INSERT INTO auth_sessions(user_id,token_hash,device_label,expires_at) VALUES($1,$2,$3,$4)",u.ID,tokenHash(token),safeLabel(label),expires)
 if err!=nil{return Session{},err}
 return Session{Token:token,ExpiresAt:expires,User:u},nil
}
func(s *Service)Authenticate(ctx context.Context,token string)(User,string,error){if token==""{return User{},"",ErrInvalidCredentials};var u User;var sid,status string;err:=s.db.QueryRowContext(ctx,`SELECT u.id::text,COALESCE(u.tenant_id::text,''),u.username,u.role,s.id::text,u.status FROM auth_sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>NOW()`,tokenHash(token)).Scan(&u.ID,&u.TenantID,&u.Username,&u.Role,&sid,&status);if err!=nil||status!="active"{return User{},"",ErrInvalidCredentials};_,_=s.db.ExecContext(ctx,`UPDATE auth_sessions SET last_seen_at=NOW() WHERE id=$1`,sid);return u,sid,nil}
func(s *Service)CanOperate(ctx context.Context,tenantID string)bool{if tenantID==""{return true};var ok bool;err:=s.db.QueryRowContext(ctx,`SELECT status='active' AND expires_at>NOW() FROM tenants WHERE id=$1`,tenantID).Scan(&ok);return err==nil&&ok}
func(s *Service)Activate(ctx context.Context,tenantID,code string)(time.Time,error){
 code=strings.TrimSpace(code);if len(code)<8||len(code)>128{return time.Time{},ErrCardInvalid}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return time.Time{},err};defer tx.Rollback()
 if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,tenantID);err!=nil{return time.Time{},err}
 var cid string;var days int
 err=tx.QueryRowContext(ctx,`SELECT id::text,duration_days FROM activation_cards WHERE code_hash=$1 AND status='unused' FOR UPDATE`,tokenHash(code)).Scan(&cid,&days)
 if err!=nil{return time.Time{},ErrCardInvalid}
 var wasInactive bool
 if err=tx.QueryRowContext(ctx,`SELECT status<>'active' OR expires_at IS NULL OR expires_at<=NOW() FROM tenants WHERE id=$1 FOR UPDATE`,tenantID).Scan(&wasInactive);err!=nil{return time.Time{},err}
 // Renewal must freeze old work before extending an expired subscription.
 // Live Telegram RPC leases are retained so their actual result can be saved.
 if wasInactive { if err=pauseInactiveTenantWork(ctx,tx,tenantID);err!=nil{return time.Time{},err} }
 var expiry time.Time
 err=tx.QueryRowContext(ctx,`UPDATE tenants SET expires_at=GREATEST(COALESCE(expires_at,NOW()),NOW()) + ($2 * INTERVAL '1 day'),status='active',updated_at=NOW() WHERE id=$1 AND status<>'disabled' RETURNING expires_at`,tenantID,days).Scan(&expiry)
 if err!=nil{return time.Time{},ErrAccountInactive}
 if _,err=tx.ExecContext(ctx,`UPDATE activation_cards SET status='activated',activated_tenant_id=$2,activated_at=NOW(),expires_at=$3 WHERE id=$1`,cid,tenantID,expiry);err!=nil{return time.Time{},err}
 if err=tx.Commit();err!=nil{return time.Time{},err}
 return expiry,nil
}
func(s *Service)Logout(ctx context.Context,sid string)error{_,err:=s.db.ExecContext(ctx,`UPDATE auth_sessions SET revoked_at=NOW() WHERE id=$1 AND revoked_at IS NULL`,sid);return err}
func(s *Service)ChangePassword(ctx context.Context,uid,current,next,sid string)error{if len(next)<10||len(next)>72{return errors.New("new password must be 10-72 bytes")};var old string;if err:=s.db.QueryRowContext(ctx,`SELECT password_hash FROM users WHERE id=$1`,uid).Scan(&old);err!=nil{return err};if bcrypt.CompareHashAndPassword([]byte(old),[]byte(current))!=nil{return ErrInvalidCredentials};hash,err:=bcrypt.GenerateFromPassword([]byte(next),bcrypt.DefaultCost);if err!=nil{return err};tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback();if _,err=tx.ExecContext(ctx,`UPDATE users SET password_hash=$2,password_changed_at=NOW() WHERE id=$1`,uid,string(hash));err!=nil{return err};if _,err=tx.ExecContext(ctx,`UPDATE auth_sessions SET revoked_at=NOW() WHERE user_id=$1 AND id<>$2 AND revoked_at IS NULL`,uid,sid);err!=nil{return err};return tx.Commit()}
func randomToken()(string,error){b:=make([]byte,32);if _,err:=rand.Read(b);err!=nil{return "",err};return base64.RawURLEncoding.EncodeToString(b),nil};func tokenHash(t string)[]byte{sum:=sha256.Sum256([]byte(t));return sum[:]};func safeLabel(v string)string{v=strings.TrimSpace(v);if len(v)>100{v=v[:100]};return v}

func (s *Service) AllowRequest(ctx context.Context, scope, remote string, limit int, window time.Duration) (bool,error) {
 if strings.TrimSpace(remote)=="" { remote="unknown" }
 key:=sha256.Sum256([]byte(scope+"\\x00"+remote))
 seconds:=int64(window.Seconds());if seconds<1{seconds=1}
 var attempts int
 err:=s.db.QueryRowContext(ctx,`
 INSERT INTO request_rate_limits(key_hash,window_started_at,attempts,updated_at)
 VALUES($1,NOW(),1,NOW())
 ON CONFLICT(key_hash) DO UPDATE SET
 attempts=CASE WHEN request_rate_limits.window_started_at <= NOW()-($2 * INTERVAL '1 second') THEN 1 ELSE request_rate_limits.attempts+1 END,
 window_started_at=CASE WHEN request_rate_limits.window_started_at <= NOW()-($2 * INTERVAL '1 second') THEN NOW() ELSE request_rate_limits.window_started_at END,
 updated_at=NOW()
 RETURNING attempts`,key[:],seconds).Scan(&attempts)
 if err!=nil{return false,err};return attempts<=limit,nil
}
func (s *Service) Audit(ctx context.Context, tenantID, actorID, action, resourceType, resourceID string, details any) error {
 raw,err:=json.Marshal(details);if err!=nil{return err}
 var tenant any
 if tenantID!="" { tenant=tenantID }
 _,err=s.db.ExecContext(ctx,`INSERT INTO audit_logs(tenant_id,actor_user_id,action,resource_type,resource_id,details) VALUES($1,$2,$3,$4,$5,$6::jsonb)`,tenant,actorID,action,resourceType,resourceID,string(raw))
 return err
}


type WorkerInstance struct {
	InstanceID string `json:"instance_id"`
	Hostname string `json:"hostname"`
	ProcessID int `json:"process_id"`
	Status string `json:"status"`
	HeartbeatAt time.Time `json:"heartbeat_at"`
	StartedAt time.Time `json:"started_at"`
	StoppedAt *time.Time `json:"stopped_at,omitempty"`
	Stale bool `json:"stale"`
}

func (s *Service) ListWorkerInstances(ctx context.Context, limit int) ([]WorkerInstance, error) {
	if limit < 1 || limit > 200 { limit = 100 }
	rows, err := s.db.QueryContext(ctx, `
		SELECT instance_id,hostname,process_id,status,heartbeat_at,started_at,stopped_at
		FROM worker_instances ORDER BY heartbeat_at DESC LIMIT $1
	`, limit)
	if err != nil { return nil, err }
	defer rows.Close()
	items := make([]WorkerInstance, 0)
	now := time.Now().UTC()
	for rows.Next() {
		var item WorkerInstance
		if err := rows.Scan(&item.InstanceID,&item.Hostname,&item.ProcessID,&item.Status,&item.HeartbeatAt,&item.StartedAt,&item.StoppedAt); err != nil { return nil, err }
		item.Stale = workerHeartbeatIsStale(item.Status, item.HeartbeatAt, now)
		if item.Stale { item.Status = "stale" }
		items = append(items,item)
	}
	return items, rows.Err()
}


func workerHeartbeatIsStale(status string, heartbeatAt, now time.Time) bool {
	return status == "running" && now.Sub(heartbeatAt) > 45*time.Second
}
