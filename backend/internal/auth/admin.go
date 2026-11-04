package auth

import (
 "context"
 "crypto/rand"
 "database/sql"
 "encoding/hex"
 "errors"
 "fmt"
 "time"

 "golang.org/x/crypto/bcrypt"
)

type CardIssue struct { ID string `json:"id"`; Code string `json:"code"`; DurationDays int `json:"duration_days"`; CreatedAt time.Time `json:"created_at"` }
type CardInfo struct { ID string `json:"id"`; DurationDays int `json:"duration_days"`; Status string `json:"status"`; TenantID string `json:"tenant_id,omitempty"`; ActivatedAt *time.Time `json:"activated_at,omitempty"`; CreatedAt time.Time `json:"created_at"` }
type TenantInfo struct { ID string `json:"id"`; Name string `json:"name"`; Status string `json:"status"`; AccountLimit int `json:"account_limit"`; ExpiresAt *time.Time `json:"expires_at,omitempty"`; CreatedAt time.Time `json:"created_at"` }

func (s *Service) EnsureSuperAdmin(ctx context.Context, username, password string) error {
 if username=="" && password=="" { return nil }
 if username=="" || len(password)<16 || len(password)>72 { return errors.New("SUPER_ADMIN_USERNAME and a 16-72 byte SUPER_ADMIN_PASSWORD must both be configured") }
 var count int
 if err:=s.db.QueryRowContext(ctx,`SELECT COUNT(*) FROM users WHERE role='super_admin'`).Scan(&count);err!=nil{return err}
 if count>0{return nil}
 hash,err:=bcrypt.GenerateFromPassword([]byte(password),bcrypt.DefaultCost);if err!=nil{return err}
 _,err=s.db.ExecContext(ctx,`INSERT INTO users(tenant_id,username,password_hash,role) VALUES(NULL,$1,$2,'super_admin')`,username,string(hash))
 return err
}

func (s *Service) IssueCards(ctx context.Context, actorID string, count, days int) ([]CardIssue,error) {
 if count<1||count>100||days<1||days>3650{return nil,errors.New("count must be 1-100 and duration_days must be 1-3650")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return nil,err};defer tx.Rollback()
 cards:=make([]CardIssue,0,count)
 for i:=0;i<count;i++{
  b:=make([]byte,18);if _,err=rand.Read(b);err!=nil{return nil,err};code:="TG-"+hex.EncodeToString(b)
  var item CardIssue
  item.Code=code;item.DurationDays=days
  err=tx.QueryRowContext(ctx,`INSERT INTO activation_cards(code_hash,duration_days,created_by) VALUES($1,$2,$3) RETURNING id::text,created_at`,tokenHash(code),days,actorID).Scan(&item.ID,&item.CreatedAt)
  if err!=nil{return nil,err};cards=append(cards,item)
 }
 if err=tx.Commit();err!=nil{return nil,err};return cards,nil
}

func (s *Service) ListCards(ctx context.Context, limit int)([]CardInfo,error){
 if limit<1||limit>500{limit=100};rows,err:=s.db.QueryContext(ctx,`SELECT id::text,duration_days,status,COALESCE(activated_tenant_id::text,''),activated_at,created_at FROM activation_cards ORDER BY created_at DESC LIMIT $1`,limit);if err!=nil{return nil,err};defer rows.Close()
 out:=[]CardInfo{};for rows.Next(){var x CardInfo;if err:=rows.Scan(&x.ID,&x.DurationDays,&x.Status,&x.TenantID,&x.ActivatedAt,&x.CreatedAt);err!=nil{return nil,err};out=append(out,x)};return out,rows.Err()
}
func (s *Service) DisableCard(ctx context.Context,id string)error{res,err:=s.db.ExecContext(ctx,`UPDATE activation_cards SET status='disabled' WHERE id=$1 AND status='unused'`,id);if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("card not found or already used/disabled")};return nil}
func (s *Service) ListTenants(ctx context.Context,limit int)([]TenantInfo,error){
 if limit<1||limit>500{limit=100};rows,err:=s.db.QueryContext(ctx,`SELECT id::text,name,status,account_limit,expires_at,created_at FROM tenants ORDER BY created_at DESC LIMIT $1`,limit);if err!=nil{return nil,err};defer rows.Close()
 out:=[]TenantInfo{};for rows.Next(){var x TenantInfo;if err:=rows.Scan(&x.ID,&x.Name,&x.Status,&x.AccountLimit,&x.ExpiresAt,&x.CreatedAt);err!=nil{return nil,err};out=append(out,x)};return out,rows.Err()
}
// pauseInactiveTenantWork freezes work that has not started and requests a pause
// for live Telegram RPCs without revoking their leases. The worker must be
// allowed to persist the actual RPC result before the task becomes paused.
func pauseInactiveTenantWork(ctx context.Context, tx *sql.Tx, tenantID string) error {
	statements := []string{
		`UPDATE message_reply_jobs SET status='paused',lease_until=NULL,last_error='tenant inactive; manual review required',updated_at=NOW()
			 WHERE tenant_id=$1 AND (status='pending' OR (status='processing' AND lease_until<=NOW()))`,
		`UPDATE keyword_action_jobs SET status='paused',lease_until=NULL,last_error='tenant inactive; manual review required',updated_at=NOW()
			 WHERE tenant_id=$1 AND (status='pending' OR (status='processing' AND lease_until<=NOW()))`,
		`UPDATE account_profile_update_jobs SET status='paused',lease_until=NULL,last_error='tenant inactive; manual review required',updated_at=NOW()
			 WHERE tenant_id=$1 AND (status='pending' OR (status='processing' AND lease_until<=NOW()))`,
		`UPDATE tenant_join_tasks SET status='paused',updated_at=NOW()
			 WHERE tenant_id=$1 AND status='queued'`,
		`UPDATE tenant_join_task_accounts SET status='paused',lease_until=NULL,pause_requested=FALSE,last_error='tenant inactive; manual review required',updated_at=NOW()
			 WHERE tenant_id=$1 AND (status='queued' OR (status='processing' AND lease_until<=NOW()))`,
		`UPDATE tenant_join_task_accounts SET pause_requested=TRUE,last_error='tenant inactive; pause requested after current Telegram operation',updated_at=NOW()
			 WHERE tenant_id=$1 AND status='processing' AND lease_until>NOW()`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement, tenantID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SetTenantStatus(ctx context.Context,id,status string)error{
 if status!="active"&&status!="disabled"{return errors.New("status must be active or disabled")}
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,id);err!=nil{return err}
 res,err:=tx.ExecContext(ctx,`UPDATE tenants SET status=$2,updated_at=NOW() WHERE id=$1`,id,status);if err!=nil{return err}
 n,_:=res.RowsAffected();if n==0{return errors.New("tenant not found")}
 if status=="disabled" {
  if err=pauseInactiveTenantWork(ctx,tx,id);err!=nil{return err}
 }
 return tx.Commit()
}
func (s *Service) SetTenantAccountLimit(ctx context.Context,id string,limit int)error{if limit<1||limit>10000{return errors.New("account_limit must be 1-10000")};res,err:=s.db.ExecContext(ctx,`UPDATE tenants SET account_limit=$2,updated_at=NOW() WHERE id=$1`,id,limit);if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("tenant not found")};return nil}
func (s *Service) SetTenantExpiry(ctx context.Context,id string,expiry time.Time)error{
 tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
 if _,err=tx.ExecContext(ctx,`SELECT set_config('app.tenant_id',$1,true)`,id);err!=nil{return err}
 var wasInactive bool
 if err=tx.QueryRowContext(ctx,`SELECT status<>'active' OR expires_at IS NULL OR expires_at<=NOW() FROM tenants WHERE id=$1 FOR UPDATE`,id).Scan(&wasInactive);err!=nil{return err}
 if wasInactive { if err=pauseInactiveTenantWork(ctx,tx,id);err!=nil{return err} }
 res,err:=tx.ExecContext(ctx,`UPDATE tenants SET expires_at=$2,status=CASE WHEN status='disabled' THEN status ELSE 'active' END,updated_at=NOW() WHERE id=$1`,id,expiry);if err!=nil{return err}
 n,_:=res.RowsAffected();if n==0{return fmt.Errorf("tenant not found")}
 if !expiry.After(time.Now()) { if err=pauseInactiveTenantWork(ctx,tx,id);err!=nil{return err} }
 return tx.Commit()
}
