package accounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type ProfileUpdateInput struct {
	AccountID string `json:"account_id"`
	FirstName string `json:"first_name"`
	LastName string `json:"last_name"`
	About string `json:"about"`
}

type ProfileUpdateJob struct {
	ID string `json:"id"`
	BatchID string `json:"batch_id"`
	TenantID string `json:"-"`
	AccountID string `json:"account_id"`
	FirstName string `json:"first_name"`
	LastName string `json:"last_name"`
	About string `json:"about"`
	Status string `json:"status"`
	Attempts int `json:"attempts"`
	NextRunAt time.Time `json:"next_run_at"`
	LeaseUntil *time.Time `json:"lease_until,omitempty"`
	LastError string `json:"last_error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func normalizeProfileUpdate(in ProfileUpdateInput) (ProfileUpdateInput,error) {
	in.AccountID=strings.TrimSpace(in.AccountID)
	in.FirstName=strings.TrimSpace(in.FirstName)
	in.LastName=strings.TrimSpace(in.LastName)
	in.About=strings.TrimSpace(in.About)
	if !validAccountUUID(in.AccountID) { return in,errors.New("invalid account id") }
	if in.FirstName=="" && in.LastName=="" && in.About=="" { return in,errors.New("at least one non-empty profile field is required") }
	if utf8.RuneCountInString(in.FirstName)>64 || utf8.RuneCountInString(in.LastName)>64 { return in,errors.New("first and last names must be at most 64 characters") }
	if utf8.RuneCountInString(in.About)>70 { return in,errors.New("profile bio must be at most 70 characters") }
	return in,nil
}

func (s *Service) CreateProfileUpdateBatch(ctx context.Context,tenantID,userID string,inputs []ProfileUpdateInput)([]ProfileUpdateJob,error) {
	if tenantID=="" || !validAccountUUID(userID) { return nil,errors.New("authenticated tenant user is required") }
	if len(inputs)<1 || len(inputs)>20 { return nil,errors.New("profile update batch must contain 1-20 accounts") }
	normalized:=make([]ProfileUpdateInput,len(inputs))
	seen:=map[string]bool{}
	for i,in:=range inputs {
		value,err:=normalizeProfileUpdate(in);if err!=nil{return nil,fmt.Errorf("account %d: %w",i+1,err)}
		if seen[value.AccountID] { return nil,errors.New("duplicate account id in profile update batch") }
		seen[value.AccountID]=true;normalized[i]=value
	}
	out:=make([]ProfileUpdateJob,0,len(normalized))
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		var active bool
		if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1 AND status='active' AND expires_at>NOW())`,tenantID).Scan(&active);err!=nil{return err}
		if !active{return errors.New("tenant subscription is inactive")}
		var batchID string
		if err:=tx.QueryRowContext(ctx,`SELECT gen_random_uuid()::text`).Scan(&batchID);err!=nil{return err}
		for _,in:=range normalized {
			var authorized bool
			if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2 AND status='authorized' AND session_ciphertext IS NOT NULL)`,tenantID,in.AccountID).Scan(&authorized);err!=nil{return err}
			if !authorized{return fmt.Errorf("account %s is not an authorized account in this tenant",in.AccountID)}
			var job ProfileUpdateJob
			err:=tx.QueryRowContext(ctx,`INSERT INTO account_profile_update_jobs(batch_id,tenant_id,account_id,requested_by_user_id,first_name,last_name,about)
				VALUES($1::uuid,$2,$3,$4,$5,$6,$7)
				RETURNING id::text,batch_id::text,tenant_id::text,account_id::text,first_name,last_name,about,status,attempts,next_run_at,lease_until,last_error,created_at,updated_at`,
				batchID,tenantID,in.AccountID,userID,in.FirstName,in.LastName,in.About).Scan(&job.ID,&job.BatchID,&job.TenantID,&job.AccountID,&job.FirstName,&job.LastName,&job.About,&job.Status,&job.Attempts,&job.NextRunAt,&job.LeaseUntil,&job.LastError,&job.CreatedAt,&job.UpdatedAt)
			if err!=nil{return err}
			out=append(out,job)
		}
		return nil
	})
	if err!=nil{return nil,err}
	return out,nil
}

func (s *Service) ListProfileUpdateJobs(ctx context.Context,tenantID,batchID,status string,limit,offset int)([]ProfileUpdateJob,int,error) {
	if limit<1||limit>100{limit=50};if offset<0{offset=0}
	if batchID!=""&&!validAccountUUID(batchID){return nil,0,errors.New("invalid batch id")}
	switch status {case "","pending","processing","paused","completed","failed":default:return nil,0,errors.New("invalid profile update status")}
	out:=[]ProfileUpdateJob{};total:=0
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		filter:=`tenant_id=$1 AND ($2='' OR batch_id=$2::uuid) AND ($3='' OR status=$3)`
		if err:=tx.QueryRowContext(ctx,`SELECT COUNT(*) FROM account_profile_update_jobs WHERE `+filter,tenantID,batchID,status).Scan(&total);err!=nil{return err}
		rows,err:=tx.QueryContext(ctx,`SELECT id::text,batch_id::text,tenant_id::text,account_id::text,first_name,last_name,about,status,attempts,next_run_at,lease_until,last_error,created_at,updated_at
			FROM account_profile_update_jobs WHERE `+filter+` ORDER BY created_at DESC,id LIMIT $4 OFFSET $5`,tenantID,batchID,status,limit,offset)
		if err!=nil{return err};defer rows.Close()
		for rows.Next(){var job ProfileUpdateJob;if err:=rows.Scan(&job.ID,&job.BatchID,&job.TenantID,&job.AccountID,&job.FirstName,&job.LastName,&job.About,&job.Status,&job.Attempts,&job.NextRunAt,&job.LeaseUntil,&job.LastError,&job.CreatedAt,&job.UpdatedAt);err!=nil{return err};out=append(out,job)}
		return rows.Err()
	})
	return out,total,err
}

func (s *Service) ClaimProfileUpdateJobs(ctx context.Context,tenantID string,limit int)([]ProfileUpdateJob,error) {
	if limit<1||limit>20{limit=10}
	out:=[]ProfileUpdateJob{}
	err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		rows,err:=tx.QueryContext(ctx,`WITH picked AS (
			SELECT id FROM account_profile_update_jobs
			WHERE tenant_id=$1 AND next_run_at<=NOW()
			  AND (status='pending' OR (status='processing' AND lease_until<NOW()))
			ORDER BY next_run_at,created_at,id
			FOR UPDATE SKIP LOCKED LIMIT $2
		)
		UPDATE account_profile_update_jobs j SET status='processing',lease_until=NOW()+INTERVAL '90 seconds',updated_at=NOW()
		FROM picked WHERE j.id=picked.id
		RETURNING j.id::text,j.batch_id::text,j.tenant_id::text,j.account_id::text,j.first_name,j.last_name,j.about,j.status,j.attempts,j.next_run_at,j.lease_until,j.last_error,j.created_at,j.updated_at`,tenantID,limit)
		if err!=nil{return err};defer rows.Close()
		for rows.Next(){var job ProfileUpdateJob;if err:=rows.Scan(&job.ID,&job.BatchID,&job.TenantID,&job.AccountID,&job.FirstName,&job.LastName,&job.About,&job.Status,&job.Attempts,&job.NextRunAt,&job.LeaseUntil,&job.LastError,&job.CreatedAt,&job.UpdatedAt);err!=nil{return err};out=append(out,job)}
		return rows.Err()
	})
	return out,err
}

func (s *Service) CompleteProfileUpdate(ctx context.Context,job ProfileUpdateJob)error {
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		res,err:=tx.ExecContext(ctx,`UPDATE account_profile_update_jobs SET status='completed',lease_until=NULL,last_error='',updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2 AND status='processing' AND lease_until IS NOT NULL`,job.TenantID,job.ID)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("profile update lease was lost")};return nil
	})
}

func (s *Service) RescheduleProfileUpdate(ctx context.Context,job ProfileUpdateJob,delay time.Duration,cause error)error {
	if delay<time.Second{delay=time.Second}
	if delay>24*time.Hour{delay=24*time.Hour}
	message:="";if cause!=nil{message=cause.Error()};if len(message)>1000{message=message[:1000]}
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		res,err:=tx.ExecContext(ctx,`UPDATE account_profile_update_jobs SET status='pending',lease_until=NULL,next_run_at=NOW()+($3 * INTERVAL '1 second'),last_error=$4,updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2 AND status='processing'`,job.TenantID,job.ID,int64(delay.Seconds()),message)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("profile update lease was lost")};return nil
	})
}

func profileUpdateRetry(attempts int)(string,time.Duration) {
	attempts++
	if attempts>=5{return "failed",0}
	delay:=30*time.Second
	for i:=1;i<attempts&&delay<15*time.Minute;i++{delay*=2}
	if delay>15*time.Minute{delay=15*time.Minute}
	return "pending",delay
}

func (s *Service) FailProfileUpdate(ctx context.Context,job ProfileUpdateJob,cause error,floodWait time.Duration)error {
	message:="profile update failed";if cause!=nil{message=cause.Error()};if len(message)>1000{message=message[:1000]}
	status:="pending";delay:=floodWait;attempts:=job.Attempts
	if floodWait<=0{status,delay=profileUpdateRetry(job.Attempts);attempts++}
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		seconds:=int64(delay/time.Second);if delay%time.Second!=0{seconds++};if seconds<1{seconds=1}
		res,err:=tx.ExecContext(ctx,`UPDATE account_profile_update_jobs SET status=$3,attempts=$4,lease_until=NULL,next_run_at=NOW()+($5 * INTERVAL '1 second'),last_error=$6,updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2 AND status='processing'`,job.TenantID,job.ID,status,attempts,seconds,message)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("profile update lease was lost")};return nil
	})
}

func (s *Service) RetryProfileUpdate(ctx context.Context,tenantID,id string)error {
	if !validAccountUUID(id){return errors.New("profile update job not found")}
	return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		res,err:=tx.ExecContext(ctx,`UPDATE account_profile_update_jobs j SET status='pending',attempts=0,next_run_at=NOW(),last_error='',updated_at=NOW()
			WHERE j.tenant_id=$1 AND j.id=$2 AND j.status='failed'
			AND EXISTS(SELECT 1 FROM tenants t WHERE t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW())
			AND EXISTS(SELECT 1 FROM telegram_accounts a WHERE a.tenant_id=j.tenant_id AND a.id=j.account_id AND a.status='authorized' AND a.session_ciphertext IS NOT NULL)`,tenantID,id)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("failed profile update job or authorized account not found")};return nil
	})
}

func maxInt64(a,b int64)int64{if a>b{return a};return b}

func validAccountUUID(value string) bool {
	if len(value)!=36{return false}
	for i,ch:=range value {
		if i==8||i==13||i==18||i==23 {if ch!='-'{return false};continue}
		if !((ch>='0'&&ch<='9')||(ch>='a'&&ch<='f')||(ch>='A'&&ch<='F')){return false}
	}
	return true
}

func (s *Service) PauseProfileUpdate(ctx context.Context,job ProfileUpdateJob,reason string)error {
	if len(reason)>1000{reason=reason[:1000]}
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		res,err:=tx.ExecContext(ctx,`UPDATE account_profile_update_jobs SET status='paused',lease_until=NULL,last_error=$3,updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2 AND status='processing'`,job.TenantID,job.ID,reason)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("profile update lease was lost")};return nil
	})
}

func (s *Service) ResumeProfileUpdate(ctx context.Context,tenantID,id string)error {
	if !validAccountUUID(id){return errors.New("profile update job not found")}
	return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
		res,err:=tx.ExecContext(ctx,`UPDATE account_profile_update_jobs j SET status='pending',attempts=0,next_run_at=NOW(),last_error='',updated_at=NOW()
			WHERE j.tenant_id=$1 AND j.id=$2 AND j.status='paused'
			AND EXISTS(SELECT 1 FROM tenants t WHERE t.id=j.tenant_id AND t.status='active' AND t.expires_at>NOW())
			AND EXISTS(SELECT 1 FROM telegram_accounts a WHERE a.tenant_id=j.tenant_id AND a.id=j.account_id AND a.status='authorized' AND a.session_ciphertext IS NOT NULL)`,tenantID,id)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("paused profile update job or authorized account not found")};return nil
	})
}

func (s *Service) PauseInactiveProfileUpdateJobs(ctx context.Context) error {
 rows,err:=s.db.QueryContext(ctx,`SELECT id::text FROM tenants`); if err!=nil{return err}
 var ids []string; for rows.Next(){var id string;if err:=rows.Scan(&id);err!=nil{rows.Close();return err};ids=append(ids,id)}
 if err:=rows.Err();err!=nil{rows.Close();return err};rows.Close()
 for _,id:=range ids { err:=withTenantTx(ctx,s.db,id,func(tx *sql.Tx)error{_,e:=tx.ExecContext(ctx,`UPDATE account_profile_update_jobs j SET status='paused',lease_until=NULL,last_error='tenant inactive; manual review required',updated_at=NOW() WHERE j.tenant_id=$1 AND j.status IN ('pending','processing') AND EXISTS(SELECT 1 FROM tenants t WHERE t.id=j.tenant_id AND (t.status<>'active' OR t.expires_at<=NOW()))`,id);return e});if err!=nil{return err} }
 return nil
}
