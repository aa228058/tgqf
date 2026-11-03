package storage

import (
 "context"
 "database/sql"
 "errors"
 "fmt"
 "time"
)

// WithTenantTx pins the tenant identity to a transaction-local PostgreSQL
// setting. RLS policies reject access when the setting is absent.
func (s *Store) WithTenantTx(ctx context.Context, tenantID string, fn func(*sql.Tx) error) error {
 if tenantID=="" { return errors.New("tenant scope is required") }
 tx,err:=s.DB.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,tenantID);err!=nil{return fmt.Errorf("set tenant scope: %w",err)}
 if err=fn(tx);err!=nil{return err}
 return tx.Commit()
}

// ConsumeRateLimit atomically allows at most limit attempts in each window.
// The caller must pass a one-way digest as keyHash, not a raw IP or credential.
func (s *Store) ConsumeRateLimit(ctx context.Context,keyHash []byte,limit int,window time.Duration)(bool,error){
 if len(keyHash)==0||limit<1||window<=0{return false,errors.New("invalid rate limit parameters")}
 seconds:=int64(window.Seconds());if seconds<1{seconds=1}
 var attempts int
 err:=s.DB.QueryRowContext(ctx,`
 INSERT INTO request_rate_limits(key_hash,window_started_at,attempts,updated_at)
 VALUES($1,NOW(),1,NOW())
 ON CONFLICT(key_hash) DO UPDATE SET
   attempts=CASE WHEN request_rate_limits.window_started_at <= NOW()-($2 * INTERVAL '1 second') THEN 1 ELSE request_rate_limits.attempts+1 END,
   window_started_at=CASE WHEN request_rate_limits.window_started_at <= NOW()-($2 * INTERVAL '1 second') THEN NOW() ELSE request_rate_limits.window_started_at END,
   updated_at=NOW()
 RETURNING attempts
 `,keyHash,seconds).Scan(&attempts)
 if err!=nil{return false,fmt.Errorf("consume rate limit: %w",err)}
 return attempts<=limit,nil
}
