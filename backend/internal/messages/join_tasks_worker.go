package messages

import (
    "context"
    "database/sql"
    "errors"
    "strings"
    "time"
)

type JoinTaskJob struct {
    TenantID string
    TaskID string
    AccountRowID int64
    AccountID string
    Attempts int
    TargetURL string
    PrivateInviteCiphertext []byte
    LeaseUntil time.Time
}

func (s *Service) ClaimJoinTaskAccounts(ctx context.Context, tenantID string, limit int) ([]JoinTaskJob,error) {
    if limit<1||limit>50 { limit=10 }
    jobs:=[]JoinTaskJob{}
    err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        rows,err:=tx.QueryContext(ctx,`
            WITH picked AS (
                SELECT a.id,a.tenant_id,a.task_id,a.telegram_account_id,a.attempts,
                    CASE WHEN l.link_type='public' THEN l.public_url WHEN NULLIF(g.username,'') IS NOT NULL THEN 'https://t.me/'||g.username END AS target_url,
                    COALESCE(t.private_invite_hash_ciphertext,l.invite_ciphertext) AS private_invite_hash_ciphertext
                FROM tenant_join_task_accounts a
                JOIN tenant_join_tasks t ON t.tenant_id=a.tenant_id AND t.id=a.task_id
                JOIN telegram_accounts acc ON acc.tenant_id=a.tenant_id AND acc.id=a.telegram_account_id
                LEFT JOIN public_group_links l ON l.id=t.target_link_id AND l.status<>'invalid'
                LEFT JOIN public_groups g ON g.id=COALESCE(t.target_group_id,l.group_id)
                WHERE a.tenant_id=$1 AND t.status='queued' AND (a.status='queued' OR (a.status='processing' AND a.lease_until<NOW()))
                    AND a.available_at<=NOW() AND t.available_at<=NOW()
                    AND (acc.join_flood_wait_until IS NULL OR acc.join_flood_wait_until<=NOW())
                    AND COALESCE(acc.telegram_send_next_at,'-infinity'::timestamptz)<=NOW()
                    AND COALESCE(acc.telegram_send_cooldown_until,'-infinity'::timestamptz)<=NOW()
                    AND acc.status='authorized' AND acc.session_ciphertext IS NOT NULL
                    AND NOT EXISTS (SELECT 1 FROM message_reply_jobs x WHERE x.tenant_id=a.tenant_id AND x.telegram_account_id=a.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
                    AND NOT EXISTS (SELECT 1 FROM keyword_action_jobs x WHERE x.tenant_id=a.tenant_id AND x.telegram_account_id=a.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
                    AND NOT EXISTS (SELECT 1 FROM account_profile_update_jobs x WHERE x.tenant_id=a.tenant_id AND x.account_id=a.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
                    AND NOT EXISTS (SELECT 1 FROM tenant_join_task_accounts x WHERE x.tenant_id=a.tenant_id AND x.telegram_account_id=a.telegram_account_id AND x.status='processing' AND x.lease_until>NOW())
                    AND (CASE WHEN l.link_type='public' THEN l.public_url WHEN NULLIF(g.username,'') IS NOT NULL THEN 'https://t.me/'||g.username END IS NOT NULL OR COALESCE(t.private_invite_hash_ciphertext,l.invite_ciphertext) IS NOT NULL)
                    AND (t.target_link_id IS NULL OR EXISTS(
                        SELECT 1 FROM public_group_link_sources src
                        WHERE src.tenant_id=a.tenant_id AND src.link_id=t.target_link_id
                    ))
                    AND EXISTS(SELECT 1 FROM tenants ten WHERE ten.id=a.tenant_id AND ten.status='active' AND ten.expires_at>NOW())
                ORDER BY a.available_at,a.created_at,a.id
                LIMIT $2 FOR UPDATE OF t,a,acc SKIP LOCKED
            ), claimed AS (
                UPDATE tenant_join_task_accounts a
                SET status='processing',lease_until=NOW()+INTERVAL '3 minutes',attempts=a.attempts+1,updated_at=NOW()
                FROM picked p
                WHERE a.id=p.id AND a.tenant_id=p.tenant_id
                RETURNING p.tenant_id,p.task_id,p.id,p.telegram_account_id,a.attempts,p.target_url,p.private_invite_hash_ciphertext,a.lease_until
            ), scheduled AS (
                UPDATE telegram_accounts acc SET telegram_send_next_at=NOW()+INTERVAL '2 seconds'
                FROM claimed c WHERE acc.tenant_id=c.tenant_id AND acc.id=c.telegram_account_id
                RETURNING acc.id
            )
            SELECT c.tenant_id::text,c.task_id::text,c.id,c.telegram_account_id::text,c.attempts,c.target_url,c.private_invite_hash_ciphertext,c.lease_until
            FROM claimed c JOIN scheduled s ON TRUE
        `,tenantID,limit)
        if err!=nil{return err};defer rows.Close()
        for rows.Next(){var job JoinTaskJob;if err:=rows.Scan(&job.TenantID,&job.TaskID,&job.AccountRowID,&job.AccountID,&job.Attempts,&job.TargetURL,&job.PrivateInviteCiphertext,&job.LeaseUntil);err!=nil{return err};if len(job.PrivateInviteCiphertext)>0 { hash,decryptErr:=decryptJoinInviteHash(job.PrivateInviteCiphertext);if decryptErr!=nil{return decryptErr};job.TargetURL="https://t.me/+"+hash };jobs=append(jobs,job)}
        return rows.Err()
    })
    return jobs,err
}

func (s *Service) CompleteJoinTaskAccount(ctx context.Context,job JoinTaskJob,chatID int64,applicationStatus string)error {
    status:="joined"
    if applicationStatus=="request_pending" {status="request_pending"}
    if applicationStatus=="already_member" {status="already_member";applicationStatus="not_required"}
    if applicationStatus!="request_pending"&&applicationStatus!="already_member"&&applicationStatus!="not_required" {return errors.New("invalid join task result")}
    return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
        if err:=lockJoinTaskParent(ctx,tx,job.TenantID,job.TaskID);err!=nil{return err}
        res,err:=tx.ExecContext(ctx,`UPDATE tenant_join_task_accounts SET status=$1,application_status=$2,telegram_chat_id=NULLIF($3,0),lease_until=NULL,last_error='',updated_at=NOW()
            WHERE tenant_id=$4 AND task_id=$5 AND id=$6 AND status='processing' AND lease_until=$7`,status,applicationStatus,chatID,job.TenantID,job.TaskID,job.AccountRowID,job.LeaseUntil)
        if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("join task lease expired or was cancelled")}
        return refreshJoinTaskStatus(ctx,tx,job.TenantID,job.TaskID)
    })
}

func joinTaskCooldownSeconds(wait time.Duration) int64 {
    if wait<=0{return 0}
    seconds:=int64(wait/time.Second)
    if wait%time.Second!=0 {seconds++}
    return seconds+1
}

func (s *Service) RescheduleJoinTaskAccount(ctx context.Context,job JoinTaskJob,wait time.Duration,reason string)error {
	seconds:=joinTaskCooldownSeconds(wait)
	if seconds<=0{return errors.New("positive FloodWait duration is required")}
	if len(reason)>500 {reason=reason[:500]}
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		if err:=lockJoinTaskParent(ctx,tx,job.TenantID,job.TaskID);err!=nil{return err}
		// Consume only this exact lease. Pause/cancel stops future work without
		// invalidating a Telegram RPC that is already running.
		res,err:=tx.ExecContext(ctx,`UPDATE tenant_join_task_accounts a SET
			status=CASE WHEN t.status='cancelled' THEN 'cancelled' WHEN t.status='paused' THEN 'paused' ELSE 'queued' END,
			available_at=CASE WHEN t.status='queued' THEN GREATEST(a.available_at,NOW()+($1 * INTERVAL '1 second')) ELSE a.available_at END,
			lease_until=NULL,last_error=$2,updated_at=NOW()
			FROM tenant_join_tasks t
			WHERE a.tenant_id=$3 AND a.task_id=$4 AND a.id=$5 AND a.status='processing' AND a.lease_until=$6
			  AND t.tenant_id=a.tenant_id AND t.id=a.task_id`,
			seconds,strings.TrimSpace(reason),job.TenantID,job.TaskID,job.AccountRowID,job.LeaseUntil)
		if err!=nil{return err}
		n,_:=res.RowsAffected()
		if n==0{return nil}
		// FloodWait is account-wide and must be respected even if the task was
		// paused or cancelled while the RPC was in flight.
		if _,err:=tx.ExecContext(ctx,`UPDATE telegram_accounts SET
			join_flood_wait_until=GREATEST(COALESCE(join_flood_wait_until,NOW()),NOW()+($1 * INTERVAL '1 second')),
			telegram_send_cooldown_until=GREATEST(COALESCE(telegram_send_cooldown_until,NOW()),NOW()+($1 * INTERVAL '1 second'))
			WHERE tenant_id=$2 AND id=$3`,seconds,job.TenantID,job.AccountID);err!=nil{return err}
		if _,err:=tx.ExecContext(ctx,`UPDATE tenant_join_tasks SET available_at=GREATEST(available_at,NOW()+($1 * INTERVAL '1 second'))
			WHERE tenant_id=$2 AND id=$3 AND status='queued'`,seconds,job.TenantID,job.TaskID);err!=nil{return err}
		// Delay queued rows only. Never release another worker's processing lease.
		_,err=tx.ExecContext(ctx,`UPDATE tenant_join_task_accounts a SET
			available_at=GREATEST(a.available_at,NOW()+($1 * INTERVAL '1 second')),updated_at=NOW()
			FROM tenant_join_tasks t
			WHERE a.tenant_id=$3 AND a.id<>$4 AND (a.telegram_account_id=$5 OR a.task_id=$6)
			  AND a.status='queued' AND t.tenant_id=a.tenant_id AND t.id=a.task_id AND t.status='queued'`,
			seconds,strings.TrimSpace(reason),job.TenantID,job.AccountRowID,job.AccountID,job.TaskID)
		return err
	})
}

func (s *Service) FailJoinTaskAccount(ctx context.Context,job JoinTaskJob,reason string)error {
    if len(reason)>500 {reason=reason[:500]}
    return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
        if err:=lockJoinTaskParent(ctx,tx,job.TenantID,job.TaskID);err!=nil{return err}
        res,err:=tx.ExecContext(ctx,`UPDATE tenant_join_task_accounts a SET
				status=CASE WHEN t.status='cancelled' THEN 'cancelled' ELSE 'failed' END,
				lease_until=NULL,last_error=$1,updated_at=NOW()
			FROM tenant_join_tasks t
			WHERE a.tenant_id=$2 AND a.task_id=$3 AND a.id=$4 AND a.status='processing' AND a.lease_until=$5
			  AND t.tenant_id=a.tenant_id AND t.id=a.task_id`,strings.TrimSpace(reason),job.TenantID,job.TaskID,job.AccountRowID,job.LeaseUntil)
        if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("join task lease expired or was cancelled")}
        return refreshJoinTaskStatus(ctx,tx,job.TenantID,job.TaskID)
    })
}


// RescheduleJoinTaskRetry retries transient Telegram failures with bounded
// exponential backoff. The exact lease is consumed before the row is requeued.
func joinTaskRetryDelay(attempt int) time.Duration {
    if attempt < 1 { attempt = 1 }
    delay := 30 * time.Second
    for i := 1; i < attempt && delay < 30*time.Minute; i++ { delay *= 2 }
    if delay > 30*time.Minute { return 30*time.Minute }
    return delay
}

func (s *Service) RescheduleJoinTaskRetry(ctx context.Context, job JoinTaskJob, reason string) error {
	if job.Attempts >= 5 {
		return s.FailJoinTaskAccount(ctx, job, "retry limit reached: "+reason)
	}
	delay := joinTaskRetryDelay(job.Attempts)
	if len(reason) > 500 { reason = reason[:500] }
	return withTenantTx(ctx, s.db, job.TenantID, func(tx *sql.Tx) error {
		if err := lockJoinTaskParent(ctx, tx, job.TenantID, job.TaskID); err != nil { return err }
		res, err := tx.ExecContext(ctx, `UPDATE tenant_join_task_accounts a SET
			status=CASE WHEN t.status='cancelled' THEN 'cancelled' WHEN t.status='paused' THEN 'paused' ELSE 'queued' END,
			available_at=CASE WHEN t.status='queued' THEN GREATEST(a.available_at,NOW()+($1 * INTERVAL '1 second')) ELSE a.available_at END,
			lease_until=NULL,last_error=$2,updated_at=NOW()
			FROM tenant_join_tasks t
			WHERE a.tenant_id=$3 AND a.task_id=$4 AND a.id=$5 AND a.status='processing' AND a.lease_until=$6
			  AND t.tenant_id=a.tenant_id AND t.id=a.task_id`,
			int64(delay/time.Second), strings.TrimSpace(reason), job.TenantID, job.TaskID, job.AccountRowID, job.LeaseUntil)
		if err != nil { return err }
		n, err := res.RowsAffected()
		if err != nil { return err }
		if n == 0 { return errors.New("join task lease expired or was cancelled") }
		_, err = tx.ExecContext(ctx, `UPDATE tenant_join_tasks SET available_at=GREATEST(available_at,NOW()+($1 * INTERVAL '1 second'))
			WHERE tenant_id=$2 AND id=$3 AND status='queued'`, int64(delay/time.Second), job.TenantID, job.TaskID)
		return err
	})
}

func lockJoinTaskParent(ctx context.Context, tx *sql.Tx, tenantID, taskID string) error {
    var lockedID string
    err := tx.QueryRowContext(ctx, `SELECT id::text FROM tenant_join_tasks WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, taskID).Scan(&lockedID)
    if errors.Is(err, sql.ErrNoRows) { return errors.New("join task not found") }
    return err
}

func refreshJoinTaskStatus(ctx context.Context,tx *sql.Tx,tenantID,taskID string)error {
    _,err:=tx.ExecContext(ctx,`
        UPDATE tenant_join_tasks t SET status=CASE
            WHEN EXISTS(SELECT 1 FROM tenant_join_task_accounts a WHERE a.tenant_id=t.tenant_id AND a.task_id=t.id AND a.status='failed') THEN 'failed'
            ELSE 'completed' END,updated_at=NOW()
        WHERE t.tenant_id=$1 AND t.id=$2 AND t.status IN ('queued','paused')
          AND NOT EXISTS(SELECT 1 FROM tenant_join_task_accounts a WHERE a.tenant_id=t.tenant_id AND a.task_id=t.id
              AND a.status NOT IN ('joined','already_member','skipped','failed','cancelled'))
    `,tenantID,taskID)
    return err
}

type JoinApprovalJob struct {
 TenantID string
 TaskID string
 AccountRowID int64
 AccountID string
 ChatID int64
 InviteHash string
 LeaseUntil time.Time
}

func (s *Service) ListPendingJoinApprovals(ctx context.Context,tenantID string,limit int)([]JoinApprovalJob,error) {
 if limit<1||limit>100 {limit=50}
 out:=[]JoinApprovalJob{}
 err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
  rows,err:=tx.QueryContext(ctx,`WITH due AS (
    SELECT a.id,a.tenant_id,a.task_id,a.telegram_account_id,
           COALESCE(a.telegram_chat_id,g.telegram_chat_id,0) AS chat_id,
           t.private_invite_hash_ciphertext
    FROM tenant_join_task_accounts a
    JOIN tenant_join_tasks t ON t.tenant_id=a.tenant_id AND t.id=a.task_id
    LEFT JOIN public_group_links l ON l.id=t.target_link_id
    LEFT JOIN public_groups g ON g.id=COALESCE(t.target_group_id,l.group_id)
    JOIN telegram_accounts acc ON acc.tenant_id=a.tenant_id AND acc.id=a.telegram_account_id
    WHERE a.tenant_id=$1 AND t.status='queued' AND a.status='request_pending'
      AND (a.approval_check_lease_until IS NULL OR a.approval_check_lease_until<=NOW())
      AND (COALESCE(a.telegram_chat_id,g.telegram_chat_id) IS NOT NULL OR t.private_invite_hash_ciphertext IS NOT NULL)
      AND acc.status='authorized' AND acc.session_ciphertext IS NOT NULL
      AND EXISTS(SELECT 1 FROM tenants ten WHERE ten.id=a.tenant_id AND ten.status='active' AND ten.expires_at>NOW())
    ORDER BY a.updated_at,a.id
    LIMIT $2 FOR UPDATE OF a SKIP LOCKED
   ), claimed AS (
    UPDATE tenant_join_task_accounts a
    SET approval_check_lease_until=NOW()+INTERVAL '2 minutes',updated_at=NOW()
    FROM due p WHERE a.id=p.id AND a.tenant_id=p.tenant_id
    RETURNING p.tenant_id,p.task_id,p.id,p.telegram_account_id,p.chat_id,p.private_invite_hash_ciphertext,a.approval_check_lease_until
   )
   SELECT tenant_id::text,task_id::text,id,telegram_account_id::text,chat_id,private_invite_hash_ciphertext,approval_check_lease_until
   FROM claimed ORDER BY id`,tenantID,limit)
  if err!=nil{return err}
  defer rows.Close()
  for rows.Next(){
   var job JoinApprovalJob
   var ciphertext []byte
   if err:=rows.Scan(&job.TenantID,&job.TaskID,&job.AccountRowID,&job.AccountID,&job.ChatID,&ciphertext,&job.LeaseUntil);err!=nil{return err}
   if len(ciphertext)>0 {
    hash,err:=decryptJoinInviteHash(ciphertext)
    if err!=nil{return err}
    job.InviteHash=hash
   }
   out=append(out,job)
  }
  return rows.Err()
 })
 return out,err
}

func (s *Service) DeferJoinApprovalCheck(ctx context.Context,job JoinApprovalJob,wait time.Duration) error {
 seconds:=joinTaskCooldownSeconds(wait)
 if seconds<=0 { return errors.New("positive approval check delay is required") }
 return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
  if err:=lockJoinTaskParent(ctx,tx,job.TenantID,job.TaskID);err!=nil{return err}
  res,err:=tx.ExecContext(ctx,`UPDATE tenant_join_task_accounts
   SET approval_check_lease_until=GREATEST(approval_check_lease_until,NOW()+($1*INTERVAL '1 second'))
   WHERE tenant_id=$2 AND task_id=$3 AND id=$4 AND status='request_pending' AND approval_check_lease_until=$5
     AND EXISTS(SELECT 1 FROM tenant_join_tasks t WHERE t.tenant_id=$2 AND t.id=$3 AND t.status='queued')`,
   seconds,job.TenantID,job.TaskID,job.AccountRowID,job.LeaseUntil)
  if err!=nil{return err}
  n,_:=res.RowsAffected()
  if n==0{return errors.New("approval check lease expired or was superseded")}
  _,err=tx.ExecContext(ctx,`UPDATE telegram_accounts SET
   join_flood_wait_until=GREATEST(COALESCE(join_flood_wait_until,NOW()),NOW()+($1*INTERVAL '1 second')),
   telegram_send_cooldown_until=GREATEST(COALESCE(telegram_send_cooldown_until,NOW()),NOW()+($1*INTERVAL '1 second'))
   WHERE tenant_id=$2 AND id=$3`,seconds,job.TenantID,job.AccountID)
  return err
 })
}

func normalizeJoinApprovalResult(memberStatus string) (string,string,bool) {
	switch strings.ToLower(strings.TrimSpace(memberStatus)) {
	case "member","administrator","creator","owner":
		return "joined","approved",true
	case "banned","left","invalid","expired":
		// Membership absence or an invalid invite is not proof that an admin
		// explicitly rejected the request. Preserve that distinction.
		return "failed","unknown",true
	default:
		return "request_pending","request_pending",false
	}
}

func (s *Service) SyncJoinApproval(ctx context.Context,job JoinApprovalJob,memberStatus string)error {
	status,application,terminal:=normalizeJoinApprovalResult(memberStatus)
	if !terminal { return nil }
	return withTenantTx(ctx,s.db,job.TenantID,func(tx *sql.Tx)error{
		if err:=lockJoinTaskParent(ctx,tx,job.TenantID,job.TaskID);err!=nil{return err}
		res,err:=tx.ExecContext(ctx,`UPDATE tenant_join_task_accounts SET status=$1,application_status=$2,approval_check_lease_until=NULL,last_error='',updated_at=NOW()
			WHERE tenant_id=$3 AND task_id=$4 AND id=$5 AND status='request_pending' AND approval_check_lease_until=$6
              AND EXISTS(SELECT 1 FROM tenant_join_tasks t WHERE t.tenant_id=$3 AND t.id=$4 AND t.status='queued')`,status,application,job.TenantID,job.TaskID,job.AccountRowID,job.LeaseUntil)
		if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return nil}
		return refreshJoinTaskStatus(ctx,tx,job.TenantID,job.TaskID)
	})
}
