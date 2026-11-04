package messages

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "strings"
    "time"
)

var ErrInvalidJoinTask = errors.New("invalid join task")

type JoinTaskInput struct {
    Name string `json:"name"`
    TargetGroupID int64 `json:"target_group_id,omitempty"`
    TargetLinkID int64 `json:"target_link_id,omitempty"`
    AccountIDs []string `json:"account_ids"`
    AuthorizationConfirmed bool `json:"authorization_confirmed"`
    AuthorizationNote string `json:"authorization_note"`
    PrivateInviteLink string `json:"private_invite_link,omitempty"`
}

type JoinTask struct {
    ID string `json:"id"`
    Name string `json:"name"`
    TargetGroupID int64 `json:"target_group_id,omitempty"`
    TargetLinkID int64 `json:"target_link_id,omitempty"`
    Status string `json:"status"`
    AuthorizationConfirmed bool `json:"authorization_confirmed"`
    AuthorizationNote string `json:"authorization_note"`
    AuthorizedAt *time.Time `json:"authorized_at,omitempty"`
    AccountCount int `json:"account_count"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type JoinTaskAccount struct {
    AccountID string `json:"account_id"`
    AccountName string `json:"account_name"`
    Status string `json:"status"`
    ApplicationStatus string `json:"application_status"`
    Attempts int `json:"attempts"`
    LastError string `json:"last_error,omitempty"`
    UpdatedAt time.Time `json:"updated_at"`
}

type JoinTaskDetail struct {
    JoinTask
    Accounts []JoinTaskAccount `json:"accounts"`
}

func normalizePrivateInviteLink(value string)(string,error) {
    value=strings.TrimSpace(value)
    value=strings.TrimPrefix(value,"https://");value=strings.TrimPrefix(value,"http://")
    value=strings.TrimPrefix(value,"www.")
    if strings.HasPrefix(value,"t.me/"){value=strings.TrimPrefix(value,"t.me/")} else if strings.HasPrefix(value,"telegram.me/"){value=strings.TrimPrefix(value,"telegram.me/")} else {return "",errors.New("unsupported invite host")}
    value=strings.Trim(value,"/")
    value=strings.TrimPrefix(value,"+")
    value=strings.TrimPrefix(value,"joinchat/")
    if strings.ContainsAny(value,"/?#&=+ ")||len(value)<8||len(value)>128{return "",errors.New("invalid invite hash")}
    for _,ch:=range value {if !((ch>='a'&&ch<='z')||(ch>='A'&&ch<='Z')||(ch>='0'&&ch<='9')||ch=='_'||ch=='-'){return "",errors.New("invalid invite hash")}}
    return value,nil
}

func normalizeJoinTaskInput(in JoinTaskInput)(JoinTaskInput,error) {
    in.Name=strings.TrimSpace(in.Name)
    in.AuthorizationNote=strings.TrimSpace(in.AuthorizationNote)
    if len(in.Name)<1||len(in.Name)>120||len(in.AuthorizationNote)>500{return in,fmt.Errorf("%w: invalid task name or authorization note",ErrInvalidJoinTask)}
    if !in.AuthorizationConfirmed{return in,fmt.Errorf("%w: explicit authorization confirmation is required",ErrInvalidJoinTask)}
    if in.PrivateInviteLink!="" {
        hash,err:=normalizePrivateInviteLink(in.PrivateInviteLink);if err!=nil{return in,fmt.Errorf("%w: invalid private invite link",ErrInvalidJoinTask)}
        in.PrivateInviteLink=hash
    }
    targetCount:=0;if in.TargetGroupID>0{targetCount++};if in.TargetLinkID>0{targetCount++};if in.PrivateInviteLink!=""{targetCount++}
    if targetCount!=1{return in,fmt.Errorf("%w: select exactly one public target or private invite link",ErrInvalidJoinTask)}
    if in.TargetGroupID<0||in.TargetLinkID<0{return in,ErrInvalidJoinTask}
    if len(in.AccountIDs)==0||len(in.AccountIDs)>20{return in,fmt.Errorf("%w: assign 1-20 accounts",ErrInvalidJoinTask)}
    seen:=map[string]bool{}
    for _,id:=range in.AccountIDs {if !validUUID(id)||seen[id]{return in,fmt.Errorf("%w: invalid or duplicate account id",ErrInvalidJoinTask)};seen[id]=true}
    return in,nil
}

func (s *Service) CreateJoinTask(ctx context.Context,tenantID,actorID string,input JoinTaskInput)(JoinTask,error) {
    input,err:=normalizeJoinTaskInput(input);if err!=nil{return JoinTask{},err}
    if !validUUID(actorID){return JoinTask{},ErrInvalidJoinTask}
    var inviteCiphertext []byte
    if input.PrivateInviteLink!="" { inviteCiphertext,err=encryptJoinInviteHash(input.PrivateInviteLink);if err!=nil{return JoinTask{},err} }
    task:=JoinTask{Name:input.Name,TargetGroupID:input.TargetGroupID,TargetLinkID:input.TargetLinkID,Status:"draft",AuthorizationConfirmed:true,AuthorizationNote:input.AuthorizationNote}
    err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        var active bool
        if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1 AND status='active' AND expires_at>NOW())`,tenantID).Scan(&active);err!=nil{return err}
        if !active{return errors.New("tenant subscription inactive")}
        if input.TargetGroupID>0 {var exists bool;if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM public_groups WHERE id=$1 AND NULLIF(username,'') IS NOT NULL)",input.TargetGroupID).Scan(&exists);err!=nil{return err};if !exists{return errors.New("target group not found")}}
        if input.TargetLinkID>0 {var sourceExists bool;if err:=tx.QueryRowContext(ctx,"SELECT EXISTS(SELECT 1 FROM public_group_links l JOIN public_group_link_sources s ON s.link_id=l.id AND s.tenant_id=$1 WHERE l.id=$2 AND l.link_type IN ('public','invite') AND l.status<>'invalid' AND ($3=0 OR (l.link_type='public' AND l.group_id=$3)))",tenantID,input.TargetLinkID,input.TargetGroupID).Scan(&sourceExists);err!=nil{return err};if !sourceExists{return errors.New("target link was not discovered by this tenant or is invalid")}}
        for _,accountID:=range input.AccountIDs {
            var authorized bool
            if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM telegram_accounts WHERE tenant_id=$1 AND id=$2 AND status='authorized' AND session_ciphertext IS NOT NULL)`,tenantID,accountID).Scan(&authorized);err!=nil{return err}
            if !authorized{return fmt.Errorf("account %s is not authorized for this tenant",accountID)}
        }
        err:=tx.QueryRowContext(ctx,`INSERT INTO tenant_join_tasks(tenant_id,name,target_group_id,target_link_id,private_invite_hash_ciphertext,status,authorization_confirmed,authorization_note,authorized_by,authorized_at)
            VALUES($1,$2,NULLIF($3,0),NULLIF($4,0),$5,'draft',TRUE,$6,$7,NOW())
            RETURNING id::text,created_at,updated_at,authorized_at`,tenantID,input.Name,input.TargetGroupID,input.TargetLinkID,inviteCiphertext,input.AuthorizationNote,actorID).Scan(&task.ID,&task.CreatedAt,&task.UpdatedAt,&task.AuthorizedAt)
        if err!=nil{return err}
        for _,accountID:=range input.AccountIDs {if _,err:=tx.ExecContext(ctx,"INSERT INTO tenant_join_task_accounts(tenant_id,task_id,telegram_account_id) VALUES($1,$2,$3)",tenantID,task.ID,accountID);err!=nil{return err}}
        task.AccountCount=len(input.AccountIDs)
        return nil
    })
    return task,err
}

func (s *Service) ListJoinTasks(ctx context.Context,tenantID string,limit,offset int)([]JoinTask,int,error) {
    if limit<1||limit>100{limit=50};if offset<0{offset=0}
    out:=[]JoinTask{};total:=0
    err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM tenant_join_tasks WHERE tenant_id=$1",tenantID).Scan(&total);err!=nil{return err}
        rows,err:=tx.QueryContext(ctx,`SELECT t.id::text,t.name,COALESCE(t.target_group_id,0),COALESCE(t.target_link_id,0),t.status,t.authorization_confirmed,t.authorization_note,t.authorized_at,
            (SELECT COUNT(*) FROM tenant_join_task_accounts a WHERE a.tenant_id=t.tenant_id AND a.task_id=t.id),t.created_at,t.updated_at
            FROM tenant_join_tasks t WHERE t.tenant_id=$1 ORDER BY t.created_at DESC,t.id DESC LIMIT $2 OFFSET $3`,tenantID,limit,offset)
        if err!=nil{return err};defer rows.Close()
        for rows.Next(){var item JoinTask;if err:=rows.Scan(&item.ID,&item.Name,&item.TargetGroupID,&item.TargetLinkID,&item.Status,&item.AuthorizationConfirmed,&item.AuthorizationNote,&item.AuthorizedAt,&item.AccountCount,&item.CreatedAt,&item.UpdatedAt);err!=nil{return err};out=append(out,item)}
        return rows.Err()
    })
    return out,total,err
}

func (s *Service) GetJoinTask(ctx context.Context,tenantID,id string)(JoinTaskDetail,error) {
    if !validUUID(id){return JoinTaskDetail{},errors.New("join task not found")}
    var detail JoinTaskDetail;detail.Accounts=[]JoinTaskAccount{}
    err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        err:=tx.QueryRowContext(ctx,`SELECT t.id::text,t.name,COALESCE(t.target_group_id,0),COALESCE(t.target_link_id,0),t.status,t.authorization_confirmed,t.authorization_note,t.authorized_at,
            (SELECT COUNT(*) FROM tenant_join_task_accounts a WHERE a.tenant_id=t.tenant_id AND a.task_id=t.id),t.created_at,t.updated_at
            FROM tenant_join_tasks t WHERE t.tenant_id=$1 AND t.id=$2`,tenantID,id).Scan(&detail.ID,&detail.Name,&detail.TargetGroupID,&detail.TargetLinkID,&detail.Status,&detail.AuthorizationConfirmed,&detail.AuthorizationNote,&detail.AuthorizedAt,&detail.AccountCount,&detail.CreatedAt,&detail.UpdatedAt)
        if errors.Is(err,sql.ErrNoRows){return errors.New("join task not found")};if err!=nil{return err}
        rows,err:=tx.QueryContext(ctx,`SELECT a.telegram_account_id::text,COALESCE(NULLIF(t.display_name,''),t.phone,''),a.status,a.application_status,a.attempts,a.last_error,a.updated_at
            FROM tenant_join_task_accounts a JOIN telegram_accounts t ON t.tenant_id=a.tenant_id AND t.id=a.telegram_account_id
            WHERE a.tenant_id=$1 AND a.task_id=$2 ORDER BY a.created_at,a.id`,tenantID,id)
        if err!=nil{return err};defer rows.Close()
        for rows.Next(){var item JoinTaskAccount;if err:=rows.Scan(&item.AccountID,&item.AccountName,&item.Status,&item.ApplicationStatus,&item.Attempts,&item.LastError,&item.UpdatedAt);err!=nil{return err};detail.Accounts=append(detail.Accounts,item)}
        return rows.Err()
    })
    return detail,err
}

func ensureJoinTaskAccountsAuthorized(ctx context.Context,tx *sql.Tx,tenantID,taskID string)error {
    var total,authorized int
    if err:=tx.QueryRowContext(ctx,"SELECT COUNT(*) FROM tenant_join_task_accounts WHERE tenant_id=$1 AND task_id=$2",tenantID,taskID).Scan(&total);err!=nil{return err}
    if total==0{return errors.New("join task has no assigned accounts")}
    if err:=tx.QueryRowContext(ctx,`SELECT COUNT(*) FROM tenant_join_task_accounts j JOIN telegram_accounts a ON a.tenant_id=j.tenant_id AND a.id=j.telegram_account_id
        WHERE j.tenant_id=$1 AND j.task_id=$2 AND a.status='authorized' AND a.session_ciphertext IS NOT NULL`,tenantID,taskID).Scan(&authorized);err!=nil{return err}
    if total!=authorized{return errors.New("one or more assigned accounts are no longer authorized")}
    return nil
}

func (s *Service) StartJoinTask(ctx context.Context,tenantID,id string)error {
    if !validUUID(id){return errors.New("join task not found")}
    return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        var active bool
        if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1 AND status='active' AND expires_at>NOW())`,tenantID).Scan(&active);err!=nil{return err}
        if !active{return errors.New("tenant subscription inactive")}
        if err:=ensureJoinTaskAccountsAuthorized(ctx,tx,tenantID,id);err!=nil{return err}
        res,err:=tx.ExecContext(ctx,"UPDATE tenant_join_tasks SET status='queued',updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='draft'",tenantID,id)
        if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("join task cannot be started")}
        _,err=tx.ExecContext(ctx,"UPDATE tenant_join_task_accounts SET status='queued',updated_at=NOW() WHERE tenant_id=$1 AND task_id=$2 AND status='paused'",tenantID,id)
        return err
    })
}

func (s *Service) PauseJoinTask(ctx context.Context,tenantID,id string)error {
    if !validUUID(id){return errors.New("join task not found")}
    return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        res,err:=tx.ExecContext(ctx,"UPDATE tenant_join_tasks SET status='paused',updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status IN ('draft','queued')",tenantID,id)
        if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("join task cannot be paused")}
        _,err=tx.ExecContext(ctx,"UPDATE tenant_join_task_accounts SET status='paused',lease_until=NULL,updated_at=NOW() WHERE tenant_id=$1 AND task_id=$2 AND status IN ('queued','processing')",tenantID,id)
        return err
    })
}

func (s *Service) ResumeJoinTask(ctx context.Context,tenantID,id string)error {
    if !validUUID(id){return errors.New("join task not found")}
    return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        var active bool
        if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM tenants WHERE id=$1 AND status='active' AND expires_at>NOW())`,tenantID).Scan(&active);err!=nil{return err}
        if !active{return errors.New("tenant subscription inactive")}
        if err:=ensureJoinTaskAccountsAuthorized(ctx,tx,tenantID,id);err!=nil{return err}
        res,err:=tx.ExecContext(ctx,"UPDATE tenant_join_tasks SET status='queued',updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status='paused'",tenantID,id)
        if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("join task cannot be resumed")}
        _,err=tx.ExecContext(ctx,"UPDATE tenant_join_task_accounts SET status='queued',updated_at=NOW() WHERE tenant_id=$1 AND task_id=$2 AND status='paused'",tenantID,id)
        return err
    })
}

func (s *Service) CancelJoinTask(ctx context.Context,tenantID,id string)error {
    if !validUUID(id){return errors.New("join task not found")}
    return withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        res,err:=tx.ExecContext(ctx,"UPDATE tenant_join_tasks SET status='cancelled',updated_at=NOW() WHERE tenant_id=$1 AND id=$2 AND status IN ('draft','queued','paused')",tenantID,id)
        if err!=nil{return err};n,_:=res.RowsAffected();if n==0{return errors.New("join task cannot be cancelled")}
        _,err=tx.ExecContext(ctx,"UPDATE tenant_join_task_accounts SET status='cancelled',lease_until=NULL,updated_at=NOW() WHERE tenant_id=$1 AND task_id=$2 AND status IN ('queued','paused','processing','request_pending')",tenantID,id)
        if err!=nil{return err}
        return err
    })
}


// RetryJoinTask requeues only failed account rows whose Telegram accounts are
// still authorized. The explicit task authorization must remain present.
func (s *Service) RetryJoinTask(ctx context.Context, tenantID, id string) error {
	if !validUUID(id) { return errors.New("join task not found") }
	return withTenantTx(ctx, s.db, tenantID, func(tx *sql.Tx) error {
		if err := lockJoinTaskParent(ctx, tx, tenantID, id); err != nil { return err }
		result, err := tx.ExecContext(ctx, `UPDATE tenant_join_task_accounts ja
			SET status='queued', attempts=0, available_at=NOW(), lease_until=NULL,
				last_error='', updated_at=NOW()
			FROM telegram_accounts a, tenants t
			WHERE ja.tenant_id=$1 AND ja.task_id=$2::uuid AND ja.status='failed'
			  AND a.tenant_id=ja.tenant_id AND a.id=ja.telegram_account_id
			  AND a.status='authorized' AND a.session_ciphertext IS NOT NULL
			  AND t.id=ja.tenant_id AND t.status='active' AND t.expires_at>NOW()`, tenantID, id)
		if err != nil { return err }
		count, err := result.RowsAffected()
		if err != nil { return err }
		if count == 0 { return errors.New("no failed join accounts are eligible for retry") }
		result, err = tx.ExecContext(ctx, `UPDATE tenant_join_tasks SET status='queued', updated_at=NOW()
			WHERE tenant_id=$1 AND id=$2::uuid AND status='failed'
			  AND authorization_confirmed=TRUE AND authorized_by IS NOT NULL AND authorized_at IS NOT NULL`, tenantID, id)
		if err != nil { return err }
		count, err = result.RowsAffected()
		if err != nil { return err }
		if count == 0 { return errors.New("join task is not in a retryable state") }
		return nil
	})
}
