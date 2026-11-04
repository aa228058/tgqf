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
    "unicode/utf8"
)

var (
    ErrInvalidGroupCorrection = errors.New("invalid public group correction")
    ErrGroupCorrectionNotFound = errors.New("public group correction not found")
    ErrGroupMergeConflict = errors.New("public groups cannot be merged")
)

type GroupCorrectionFields struct {
    Title *string `json:"title,omitempty"`
    Username *string `json:"username,omitempty"`
    MemberCount *int `json:"member_count,omitempty"`
    IsVerified *bool `json:"is_verified,omitempty"`
}

type GroupCorrection struct {
    ID int64 `json:"id"`
    GroupID int64 `json:"group_id"`
    TenantID string `json:"tenant_id,omitempty"`
    TenantName string `json:"tenant_name,omitempty"`
    SubmittedBy string `json:"submitted_by,omitempty"`
    ProposedFields GroupCorrectionFields `json:"proposed_fields"`
    Status string `json:"status"`
    ReviewNote string `json:"review_note"`
    ReviewedBy string `json:"reviewed_by,omitempty"`
    ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
    CreatedAt time.Time `json:"created_at"`
}

var telegramUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{5,32}$`)

func normalizeGroupCorrectionFields(in GroupCorrectionFields) (GroupCorrectionFields, []byte, error) {
    if in.Title==nil && in.Username==nil && in.MemberCount==nil && in.IsVerified==nil {
        return in,nil,ErrInvalidGroupCorrection
    }
    if in.Title!=nil {
        v:=strings.TrimSpace(*in.Title)
        if v=="" || utf8.RuneCountInString(v)>128 { return in,nil,ErrInvalidGroupCorrection }
        in.Title=&v
    }
    if in.Username!=nil {
        v:=strings.TrimPrefix(strings.TrimSpace(*in.Username),"@")
        if v!="" && !telegramUsernamePattern.MatchString(v) { return in,nil,ErrInvalidGroupCorrection }
        in.Username=&v
    }
    if in.MemberCount!=nil && (*in.MemberCount<0 || *in.MemberCount>2_000_000_000) { return in,nil,ErrInvalidGroupCorrection }
    raw,err:=json.Marshal(in)
    if err!=nil { return in,nil,err }
    return in,raw,nil
}

func (s *Service) SubmitGroupCorrection(ctx context.Context,tenantID,userID string,groupID int64,fields GroupCorrectionFields)(GroupCorrection,error) {
    if groupID<=0 { return GroupCorrection{},ErrInvalidGroupCorrection }
    fields,raw,err:=normalizeGroupCorrectionFields(fields)
    if err!=nil { return GroupCorrection{},err }
    var out GroupCorrection
    err=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        var exists bool
        if err:=tx.QueryRowContext(ctx,`SELECT EXISTS(SELECT 1 FROM public_groups WHERE id=$1)`,groupID).Scan(&exists);err!=nil{return err}
        if !exists{return ErrGroupCorrectionNotFound}
        return tx.QueryRowContext(ctx,`INSERT INTO public_group_corrections(group_id,tenant_id,submitted_by_user_id,proposed_fields)
            VALUES($1,$2,$3,$4::jsonb)
            ON CONFLICT (group_id,tenant_id) WHERE status='pending'
            DO UPDATE SET proposed_fields=EXCLUDED.proposed_fields,submitted_by_user_id=EXCLUDED.submitted_by_user_id,created_at=NOW()
            RETURNING id,group_id,tenant_id::text,proposed_fields,status,review_note,created_at`,
            groupID,tenantID,userID,string(raw)).Scan(&out.ID,&out.GroupID,&out.TenantID,&raw,&out.Status,&out.ReviewNote,&out.CreatedAt)
    })
    if err!=nil{return GroupCorrection{},err}
    if err:=json.Unmarshal(raw,&out.ProposedFields);err!=nil{return GroupCorrection{},err}
    return out,nil
}

func (s *Service) ListTenantGroupCorrections(ctx context.Context,tenantID string,limit,offset int)([]GroupCorrection,int,error) {
    if limit<1||limit>100{limit=50};if offset<0{offset=0}
    out:=[]GroupCorrection{};total:=0
    err:=withTenantTx(ctx,s.db,tenantID,func(tx *sql.Tx)error{
        if err:=tx.QueryRowContext(ctx,`SELECT COUNT(*) FROM public_group_corrections WHERE tenant_id=$1`,tenantID).Scan(&total);err!=nil{return err}
        rows,err:=tx.QueryContext(ctx,`SELECT id,group_id,tenant_id::text,proposed_fields,status,review_note,reviewed_at,created_at
            FROM public_group_corrections WHERE tenant_id=$1 ORDER BY created_at DESC,id DESC LIMIT $2 OFFSET $3`,tenantID,limit,offset)
        if err!=nil{return err};defer rows.Close()
        for rows.Next(){var item GroupCorrection;var raw []byte;if err:=rows.Scan(&item.ID,&item.GroupID,&item.TenantID,&raw,&item.Status,&item.ReviewNote,&item.ReviewedAt,&item.CreatedAt);err!=nil{return err};if err:=json.Unmarshal(raw,&item.ProposedFields);err!=nil{return err};out=append(out,item)}
        return rows.Err()
    })
    return out,total,err
}

func (s *Service) ListAdminGroupCorrections(ctx context.Context,status string,limit,offset int)([]GroupCorrection,int,error) {
    status=strings.TrimSpace(status)
    if status!=""&&status!="pending"&&status!="approved"&&status!="rejected"{return nil,0,ErrInvalidGroupCorrection}
    if limit<1||limit>100{limit=50};if offset<0{offset=0}
    out:=[]GroupCorrection{};total:=0
    args:=[]any{};filter:="1=1";if status!=""{args=append(args,status);filter+=" AND c.status=$1"}
    if err:=s.db.QueryRowContext(ctx,`SELECT COUNT(*) FROM public_group_corrections c WHERE `+filter,args...).Scan(&total);err!=nil{return nil,0,err}
    args=append(args,limit,offset);lim:=len(args)-1;off:=len(args)
    rows,err:=s.db.QueryContext(ctx,`SELECT c.id,c.group_id,c.tenant_id::text,t.name,u.username,c.proposed_fields,c.status,c.review_note,c.reviewed_by_user_id::text,c.reviewed_at,c.created_at
        FROM public_group_corrections c JOIN tenants t ON t.id=c.tenant_id JOIN users u ON u.id=c.submitted_by_user_id
        WHERE `+filter+` ORDER BY CASE WHEN c.status='pending' THEN 0 ELSE 1 END,c.created_at,c.id LIMIT $`+fmt.Sprint(lim)+` OFFSET $`+fmt.Sprint(off),args...)
    if err!=nil{return nil,0,err};defer rows.Close()
    for rows.Next(){var item GroupCorrection;var raw []byte;var reviewer sql.NullString;var reviewed sql.NullTime
        if err:=rows.Scan(&item.ID,&item.GroupID,&item.TenantID,&item.TenantName,&item.SubmittedBy,&raw,&item.Status,&item.ReviewNote,&reviewer,&reviewed,&item.CreatedAt);err!=nil{return nil,0,err}
        if err:=json.Unmarshal(raw,&item.ProposedFields);err!=nil{return nil,0,err}
        if reviewer.Valid{item.ReviewedBy=reviewer.String};if reviewed.Valid{item.ReviewedAt=&reviewed.Time}
        out=append(out,item)
    }
    return out,total,rows.Err()
}

func (s *Service) ReviewGroupCorrection(ctx context.Context,reviewerID string,correctionID int64,decision,note string)error {
    decision=strings.ToLower(strings.TrimSpace(decision));note=strings.TrimSpace(note)
    if correctionID<=0||(decision!="approve"&&decision!="reject")||len(note)>500{return ErrInvalidGroupCorrection}
    tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
    var groupID int64;var raw []byte
    err=tx.QueryRowContext(ctx,`SELECT group_id,proposed_fields FROM public_group_corrections WHERE id=$1 AND status='pending' FOR UPDATE`,correctionID).Scan(&groupID,&raw)
    if errors.Is(err,sql.ErrNoRows){return ErrGroupCorrectionNotFound};if err!=nil{return err}
    if decision=="approve" {
        if _,err=tx.ExecContext(ctx,`UPDATE public_groups SET
            title=CASE WHEN $1::jsonb ? 'title' THEN $1::jsonb->>'title' ELSE title END,
            username=CASE WHEN $1::jsonb ? 'username' THEN $1::jsonb->>'username' ELSE username END,
            member_count=CASE WHEN $1::jsonb ? 'member_count' THEN ($1::jsonb->>'member_count')::integer ELSE member_count END,
            is_verified=CASE WHEN $1::jsonb ? 'is_verified' THEN ($1::jsonb->>'is_verified')::boolean ELSE is_verified END,
            updated_at=NOW() WHERE id=$2`,string(raw),groupID);err!=nil{return err}
    }
    status:="rejected";if decision=="approve"{status="approved"}
    if _,err=tx.ExecContext(ctx,`UPDATE public_group_corrections SET status=$1,review_note=$2,reviewed_by_user_id=$3,reviewed_at=NOW() WHERE id=$4`,status,note,reviewerID,correctionID);err!=nil{return err}
    auditDetails,err:=json.Marshal(map[string]any{"group_id":groupID,"note":note,"decision":decision})
    if err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`INSERT INTO audit_logs(actor_user_id,action,resource_type,resource_id,details)
        VALUES($1,$2,'public_group_correction',$3,$4::jsonb)`,reviewerID,"admin.group_correction."+decision,fmt.Sprint(correctionID),string(auditDetails));err!=nil{return err}
    return tx.Commit()
}

func (s *Service) MergePublicGroups(ctx context.Context,actorID string,sourceID,targetID int64,reason string)error {
    reason=strings.TrimSpace(reason)
    if sourceID<=0||targetID<=0||sourceID==targetID||len(reason)<5||len(reason)>500{return ErrGroupMergeConflict}
    tx,err:=s.db.BeginTx(ctx,nil);if err!=nil{return err};defer tx.Rollback()
    var sourceType,targetType string;var snapshot []byte
    err=tx.QueryRowContext(ctx,`SELECT jsonb_build_object('telegram_chat_id',telegram_chat_id,'chat_type',chat_type,'title',title,'username',username,'member_count',member_count,'is_verified',is_verified,'default_permissions',default_permissions,'permissions_observed_at',permissions_observed_at),chat_type
        FROM public_groups WHERE id=$1 FOR UPDATE`,sourceID).Scan(&snapshot,&sourceType)
    if errors.Is(err,sql.ErrNoRows){return ErrGroupMergeConflict};if err!=nil{return err}
    err=tx.QueryRowContext(ctx,`SELECT chat_type FROM public_groups WHERE id=$1 FOR UPDATE`,targetID).Scan(&targetType)
    if errors.Is(err,sql.ErrNoRows){return ErrGroupMergeConflict};if err!=nil{return err}
    if sourceType!=targetType{return ErrGroupMergeConflict}
    if _,err=tx.ExecContext(ctx,`INSERT INTO public_group_sources(group_id,tenant_id,telegram_account_id,source_chat_id,account_rights_snapshot,account_rights_observed_at,first_seen_at,last_seen_at,discovery_count)
        SELECT $2,tenant_id,telegram_account_id,source_chat_id,account_rights_snapshot,account_rights_observed_at,first_seen_at,last_seen_at,discovery_count FROM public_group_sources WHERE group_id=$1
        ON CONFLICT(group_id,tenant_id,telegram_account_id) DO UPDATE SET first_seen_at=LEAST(public_group_sources.first_seen_at,EXCLUDED.first_seen_at),last_seen_at=GREATEST(public_group_sources.last_seen_at,EXCLUDED.last_seen_at),discovery_count=public_group_sources.discovery_count+EXCLUDED.discovery_count,
        account_rights_snapshot=COALESCE(EXCLUDED.account_rights_snapshot,public_group_sources.account_rights_snapshot),
        account_rights_observed_at=CASE WHEN EXCLUDED.account_rights_snapshot IS NULL THEN public_group_sources.account_rights_observed_at WHEN public_group_sources.account_rights_observed_at IS NULL THEN EXCLUDED.account_rights_observed_at ELSE GREATEST(public_group_sources.account_rights_observed_at,EXCLUDED.account_rights_observed_at) END`,sourceID,targetID);err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`DELETE FROM public_group_sources WHERE group_id=$1`,sourceID);err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`INSERT INTO tenant_group_contributions(tenant_id,group_id,first_contributed_at,last_contributed_at,contribution_count)
        SELECT tenant_id,$2,first_contributed_at,last_contributed_at,contribution_count FROM tenant_group_contributions WHERE group_id=$1
        ON CONFLICT(tenant_id,group_id) DO UPDATE SET first_contributed_at=LEAST(tenant_group_contributions.first_contributed_at,EXCLUDED.first_contributed_at),last_contributed_at=GREATEST(tenant_group_contributions.last_contributed_at,EXCLUDED.last_contributed_at),contribution_count=tenant_group_contributions.contribution_count+EXCLUDED.contribution_count`,sourceID,targetID);err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`DELETE FROM tenant_group_contributions WHERE group_id=$1`,sourceID);err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`UPDATE public_group_links SET group_id=$2 WHERE group_id=$1`,sourceID,targetID);err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`UPDATE public_groups target SET default_permissions=COALESCE(target.default_permissions,source.default_permissions),permissions_observed_at=CASE WHEN target.permissions_observed_at IS NULL THEN source.permissions_observed_at WHEN source.permissions_observed_at IS NULL THEN target.permissions_observed_at ELSE GREATEST(target.permissions_observed_at,source.permissions_observed_at) END,last_seen_at=GREATEST(target.last_seen_at,source.last_seen_at),updated_at=NOW() FROM public_groups source WHERE source.id=$1 AND target.id=$2`,sourceID,targetID);err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`UPDATE public_group_corrections SET status='rejected',review_note='source group merged; please resubmit against the canonical group',reviewed_by_user_id=$2,reviewed_at=NOW() WHERE group_id=$1 AND status='pending'`,sourceID,actorID);err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`UPDATE public_group_corrections SET group_id=$2 WHERE group_id=$1`,sourceID,targetID);err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`INSERT INTO public_group_merge_audits(source_group_id,target_group_id,actor_user_id,reason,source_snapshot) VALUES($1,$2,$3,$4,$5::jsonb)`,sourceID,targetID,actorID,reason,string(snapshot));err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`DELETE FROM public_groups WHERE id=$1`,sourceID);err!=nil{return err}
    auditDetails,err:=json.Marshal(map[string]any{"source_group_id":sourceID,"target_group_id":targetID,"reason":reason})
    if err!=nil{return err}
    if _,err=tx.ExecContext(ctx,`INSERT INTO audit_logs(actor_user_id,action,resource_type,resource_id,details)
        VALUES($1,'admin.public_group.merge','public_group',$2,$3::jsonb)`,actorID,fmt.Sprint(targetID),string(auditDetails));err!=nil{return err}
    return tx.Commit()
}
