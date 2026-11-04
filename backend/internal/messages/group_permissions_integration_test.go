package messages

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func openGroupIntegrationDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" { t.Skip("TEST_DATABASE_URL is not configured") }
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil { t.Fatal(err) }
	adminDB := stdlib.OpenDB(*adminConfig)
	schema := fmt.Sprintf("tgqf_groups_%d",time.Now().UnixNano())
	if _,err:=adminDB.Exec("CREATE SCHEMA "+schema);err!=nil{adminDB.Close();t.Fatal(err)}
	config,err:=pgx.ParseConfig(dsn)
	if err!=nil{adminDB.Exec("DROP SCHEMA "+schema+" CASCADE");adminDB.Close();t.Fatal(err)}
	config.RuntimeParams["search_path"]=schema
	db:=stdlib.OpenDB(*config)
	db.SetMaxOpenConns(8)
	ddl:=[]string{
		`CREATE TABLE public_groups (
			id BIGSERIAL PRIMARY KEY, telegram_chat_id BIGINT NOT NULL, chat_type TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '', member_count INTEGER NOT NULL DEFAULT 0,
			is_verified BOOLEAN NOT NULL DEFAULT FALSE, default_permissions JSONB, permissions_observed_at TIMESTAMPTZ,
			first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE(chat_type,telegram_chat_id))`,
		`CREATE TABLE public_group_sources (
			id BIGSERIAL PRIMARY KEY, group_id BIGINT NOT NULL REFERENCES public_groups(id), tenant_id UUID NOT NULL,
			telegram_account_id UUID NOT NULL, source_chat_id BIGINT NOT NULL,
			account_rights_snapshot JSONB, account_rights_observed_at TIMESTAMPTZ,
			first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			discovery_count BIGINT NOT NULL DEFAULT 1, rights_check_lease_until TIMESTAMPTZ,
			rights_next_check_at TIMESTAMPTZ, rights_check_attempts INTEGER NOT NULL DEFAULT 0,
			rights_check_error TEXT, UNIQUE(group_id,tenant_id,telegram_account_id))`,
		`CREATE TABLE tenant_group_contributions (
			tenant_id UUID NOT NULL, group_id BIGINT NOT NULL REFERENCES public_groups(id),
			first_contributed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), last_contributed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			contribution_count BIGINT NOT NULL DEFAULT 1, PRIMARY KEY(tenant_id,group_id))`,
		`CREATE TABLE public_group_links (
			id BIGSERIAL PRIMARY KEY, group_id BIGINT, link_type TEXT NOT NULL, public_url TEXT UNIQUE,
			invite_hash BYTEA UNIQUE, invite_ciphertext BYTEA, status TEXT NOT NULL DEFAULT 'pending',
			first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_checked_at TIMESTAMPTZ, last_check_error TEXT, check_attempts INTEGER NOT NULL DEFAULT 0,
			next_check_at TIMESTAMPTZ, check_lease_until TIMESTAMPTZ, check_lease_tenant_id UUID, check_lease_account_id UUID)`,
		`CREATE TABLE public_group_link_sources (
			link_id BIGINT NOT NULL, tenant_id UUID NOT NULL, telegram_account_id UUID NOT NULL,
			source_chat_id BIGINT NOT NULL, source_message_id BIGINT NOT NULL,
			first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(), last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			discovery_count BIGINT NOT NULL DEFAULT 1,
			PRIMARY KEY(link_id,tenant_id,telegram_account_id,source_chat_id,source_message_id))`,
		`CREATE TABLE tenants (id UUID PRIMARY KEY,status TEXT NOT NULL,expires_at TIMESTAMPTZ NOT NULL)`,
		`CREATE TABLE telegram_accounts (
			tenant_id UUID NOT NULL,id UUID NOT NULL,status TEXT NOT NULL,session_ciphertext BYTEA,
			join_flood_wait_until TIMESTAMPTZ,telegram_send_next_at TIMESTAMPTZ,telegram_send_cooldown_until TIMESTAMPTZ,
			PRIMARY KEY(tenant_id,id))`,
		`CREATE TABLE tenant_join_tasks (
			id UUID PRIMARY KEY,tenant_id UUID NOT NULL,name TEXT NOT NULL,status TEXT NOT NULL,
			available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW())`,
		`CREATE TABLE tenant_join_task_accounts (
			id BIGSERIAL PRIMARY KEY,tenant_id UUID NOT NULL,task_id UUID NOT NULL,telegram_account_id UUID NOT NULL,
			status TEXT NOT NULL,application_status TEXT NOT NULL DEFAULT 'not_started',attempts INTEGER NOT NULL DEFAULT 0,
			available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),lease_until TIMESTAMPTZ,telegram_chat_id BIGINT,
			last_error TEXT NOT NULL DEFAULT '',updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			approval_check_lease_until TIMESTAMPTZ,pause_requested BOOLEAN NOT NULL DEFAULT FALSE,cancel_requested BOOLEAN NOT NULL DEFAULT FALSE)`,
	}
	for _,statement:=range ddl { if _,err:=db.Exec(statement);err!=nil{db.Close();adminDB.Exec("DROP SCHEMA "+schema+" CASCADE");adminDB.Close();t.Fatal(err)} }
	cleanup:=func(){db.Close();adminDB.Exec("DROP SCHEMA "+schema+" CASCADE");adminDB.Close()}
	return db,cleanup
}

func TestGroupCatalogueImportAndPermissionRefreshIntegration(t *testing.T) {
	db,cleanup:=openGroupIntegrationDB(t)
	defer cleanup()
	ctx:=context.Background()
	service:=New(db)
	tenantA:="11111111-1111-4111-8111-111111111111"
	tenantB:="22222222-2222-4222-8222-222222222222"
	accountA:="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	accountB:="bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"

	if _,err:=db.ExecContext(ctx,`INSERT INTO public_groups(telegram_chat_id,chat_type,title) VALUES(100,'group','Original')`);err!=nil{t.Fatal(err)}
	inserted,skipped,err:=service.ImportGroupCatalogue(ctx,tenantA,[]GroupCatalogueImportRecord{
		{ChatID:100,ChatType:"group",Title:"Must Not Replace"},
		{ChatID:200,ChatType:"channel",Title:"Imported Channel",Username:"news_room",MemberCount:50},
	})
	if err!=nil{t.Fatal(err)}
	if inserted!=1||skipped!=1{t.Fatalf("inserted=%d skipped=%d, want 1/1",inserted,skipped)}
	var title string
	if err:=db.QueryRowContext(ctx,`SELECT title FROM public_groups WHERE telegram_chat_id=100 AND chat_type='group'`).Scan(&title);err!=nil{t.Fatal(err)}
	if title!="Original"{t.Fatalf("existing shared record was overwritten: %q",title)}

	var groupID int64
	if err:=db.QueryRowContext(ctx,`SELECT id FROM public_groups WHERE telegram_chat_id=100 AND chat_type='group'`).Scan(&groupID);err!=nil{t.Fatal(err)}
	for _,source:=range []struct{tenant,account string}{{tenantA,accountA},{tenantB,accountB}} {
		if _,err:=db.ExecContext(ctx,`INSERT INTO public_group_sources(group_id,tenant_id,telegram_account_id,source_chat_id) VALUES($1,$2,$3,999)`,groupID,source.tenant,source.account);err!=nil{t.Fatal(err)}
	}

	claimsA,err:=service.ListGroupPermissionRefreshes(ctx,tenantA,accountA,5)
	if err!=nil{t.Fatal(err)}
	claimsB,err:=service.ListGroupPermissionRefreshes(ctx,tenantB,accountB,5)
	if err!=nil{t.Fatal(err)}
	if len(claimsA)!=1||len(claimsB)!=1{t.Fatalf("source-scoped claims A=%d B=%d; both tenants should independently claim the shared group",len(claimsA),len(claimsB))}

	defaultRights:=[]byte(`{"send_messages":false}`)
	accountRightsA:=[]byte(`{"is_admin":true}`)
	accountRightsB:=[]byte(`{"is_admin":false}`)
	if err:=service.RecordGroupPermissionRefresh(ctx,tenantA,accountA,claimsA[0],defaultRights,accountRightsA,nil);err!=nil{t.Fatal(err)}
	if err:=service.RecordGroupPermissionRefresh(ctx,tenantB,accountB,claimsB[0],defaultRights,accountRightsB,nil);err!=nil{t.Fatal(err)}

	var savedTitle string
	var savedPermissions string
	if err:=db.QueryRowContext(ctx,`SELECT title,default_permissions::text FROM public_groups WHERE id=$1`,groupID).Scan(&savedTitle,&savedPermissions);err!=nil{t.Fatal(err)}
	var decodedDefault map[string]bool
	if err:=json.Unmarshal([]byte(savedPermissions),&decodedDefault);err!=nil{t.Fatal(err)}
	if decodedDefault["send_messages"]{t.Fatalf("default permissions unexpectedly allow sending: %s",savedPermissions)}
	if savedTitle!="Original"{t.Fatalf("empty refreshed title should preserve existing title: %q",savedTitle)}
	var rightsA,rightsB string
	if err:=db.QueryRowContext(ctx,`SELECT account_rights_snapshot::text FROM public_group_sources WHERE group_id=$1 AND tenant_id=$2 AND telegram_account_id=$3`,groupID,tenantA,accountA).Scan(&rightsA);err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,`SELECT account_rights_snapshot::text FROM public_group_sources WHERE group_id=$1 AND tenant_id=$2 AND telegram_account_id=$3`,groupID,tenantB,accountB).Scan(&rightsB);err!=nil{t.Fatal(err)}
	var decodedA,decodedB map[string]bool
	if err:=json.Unmarshal([]byte(rightsA),&decodedA);err!=nil{t.Fatal(err)}
	if err:=json.Unmarshal([]byte(rightsB),&decodedB);err!=nil{t.Fatal(err)}
	if !decodedA["is_admin"]||decodedB["is_admin"]{t.Fatalf("tenant rights snapshots were not isolated: A=%s B=%s",rightsA,rightsB)}

	// Expire and reclaim tenant A's lease; a delayed old worker must not write.
	if _,err:=db.ExecContext(ctx,`UPDATE public_group_sources SET rights_next_check_at=NULL,rights_check_lease_until=NOW()-INTERVAL '1 second' WHERE group_id=$1 AND tenant_id=$2 AND telegram_account_id=$3`,groupID,tenantA,accountA);err!=nil{t.Fatal(err)}
	reclaimed,err:=service.ListGroupPermissionRefreshes(ctx,tenantA,accountA,5)
	if err!=nil{t.Fatal(err)}
	if len(reclaimed)!=1{t.Fatalf("reclaimed=%d, want 1",len(reclaimed))}
	if err:=service.RecordGroupPermissionRefresh(ctx,tenantA,accountA,claimsA[0],defaultRights,accountRightsA,nil);!errors.Is(err,ErrGroupPermissionLeaseLost){t.Fatalf("stale worker write error=%v, want lease lost",err)}
	if err:=service.RecordGroupPermissionRefresh(ctx,tenantA,accountA,reclaimed[0],nil,nil,errors.New("temporary Telegram RPC failure"));err!=nil{t.Fatal(err)}
	var attempts int
	var checkError string
	var lease sql.NullTime
	if err:=db.QueryRowContext(ctx,`SELECT rights_check_attempts,rights_check_error,rights_check_lease_until FROM public_group_sources WHERE group_id=$1 AND tenant_id=$2 AND telegram_account_id=$3`,groupID,tenantA,accountA).Scan(&attempts,&checkError,&lease);err!=nil{t.Fatal(err)}

	if attempts!=1||checkError!="temporary Telegram RPC failure"||lease.Valid{t.Fatalf("failure retry state attempts=%d error=%q lease=%v",attempts,checkError,lease)}

	// Public and private link leases are global per link, while visibility and
	// claiming still require a source owned by the requesting tenant/account.
	t.Setenv("SESSION_ENCRYPTION_KEY",base64.StdEncoding.EncodeToString(make([]byte,32)))
	var publicLinkID int64
	if err:=db.QueryRowContext(ctx,`INSERT INTO public_group_links(link_type,public_url,status) VALUES('public','https://t.me/shared_group','pending') RETURNING id`).Scan(&publicLinkID);err!=nil{t.Fatal(err)}
	var inviteLinkID int64
	inviteCode:="AbCdEf123456"
	inviteCiphertext,err:=encryptInvite([]byte(inviteCode))
	if err!=nil{t.Fatal(err)}
	inviteHash:=sha256.Sum256([]byte(inviteCode))
	if err:=db.QueryRowContext(ctx,`INSERT INTO public_group_links(link_type,invite_hash,invite_ciphertext,status) VALUES('invite',$1,$2,'pending') RETURNING id`,inviteHash[:],inviteCiphertext).Scan(&inviteLinkID);err!=nil{t.Fatal(err)}
	for _,linkID:=range []int64{publicLinkID,inviteLinkID} {
		for _,source:=range []struct{tenant,account string}{{tenantA,accountA},{tenantB,accountB}} {
			if _,err:=db.ExecContext(ctx,`INSERT INTO public_group_link_sources(link_id,tenant_id,telegram_account_id,source_chat_id,source_message_id) VALUES($1,$2,$3,500,600)`,linkID,source.tenant,source.account);err!=nil{t.Fatal(err)}
		}
	}

	publicA,err:=service.ListPublicLinkChecks(ctx,tenantA,accountA,5)
	if err!=nil{t.Fatal(err)}
	publicB,err:=service.ListPublicLinkChecks(ctx,tenantB,accountB,5)
	if err!=nil{t.Fatal(err)}
	if len(publicA)!=1||len(publicB)!=0{t.Fatalf("public link lease should be exclusive across tenants: A=%d B=%d",len(publicA),len(publicB))}
	if _,err:=db.ExecContext(ctx,`UPDATE public_group_links SET next_check_at=NULL,check_lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`,publicLinkID);err!=nil{t.Fatal(err)}
	publicB,err=service.ListPublicLinkChecks(ctx,tenantB,accountB,5)
	if err!=nil{t.Fatal(err)}
	if len(publicB)!=1{t.Fatalf("public link expired lease was not reclaimed: %d",len(publicB))}
	if err:=service.RecordPublicLinkCheck(ctx,tenantA,accountA,publicLinkID,"valid","",publicA[0].LeaseUntil);err==nil{t.Fatal("stale public-link worker unexpectedly wrote after lease reclaim")}
	if err:=service.RecordPublicLinkCheck(ctx,tenantB,accountB,publicLinkID,"valid","",publicB[0].LeaseUntil);err!=nil{t.Fatal(err)}
	var publicStatus string
	if err:=db.QueryRowContext(ctx,`SELECT status FROM public_group_links WHERE id=$1`,publicLinkID).Scan(&publicStatus);err!=nil{t.Fatal(err)}
	if publicStatus!="valid"{t.Fatalf("public link status=%q",publicStatus)}

	privateA,err:=service.ListPrivateInviteChecks(ctx,tenantA,accountA,5)
	if err!=nil{t.Fatal(err)}
	privateB,err:=service.ListPrivateInviteChecks(ctx,tenantB,accountB,5)
	if err!=nil{t.Fatal(err)}
	if len(privateA)!=1||privateA[0].Hash!=inviteCode||len(privateB)!=0{t.Fatalf("private invite lease/decryption mismatch: A=%#v B=%#v",privateA,privateB)}
	if _,err:=db.ExecContext(ctx,`UPDATE public_group_links SET next_check_at=NULL,check_lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`,inviteLinkID);err!=nil{t.Fatal(err)}
	privateB,err=service.ListPrivateInviteChecks(ctx,tenantB,accountB,5)
	if err!=nil{t.Fatal(err)}
	if len(privateB)!=1{t.Fatalf("private invite expired lease was not reclaimed: %d",len(privateB))}
	if err:=service.RecordPrivateInviteCheck(ctx,tenantA,accountA,inviteLinkID,"valid","",privateA[0].LeaseUntil);err==nil{t.Fatal("stale private-invite worker unexpectedly wrote after lease reclaim")}
	if err:=service.RecordPrivateInviteCheck(ctx,tenantB,accountB,inviteLinkID,"approval_required","awaiting admin",privateB[0].LeaseUntil);err!=nil{t.Fatal(err)}
	var privateStatus string
	if err:=db.QueryRowContext(ctx,`SELECT status FROM public_group_links WHERE id=$1`,inviteLinkID).Scan(&privateStatus);err!=nil{t.Fatal(err)}
	if privateStatus!="approval_required"{t.Fatalf("private invite status=%q",privateStatus)}
}


func TestLinkRediscoveryPreservesActiveCheckLeases(t *testing.T) {
	db,cleanup:=openGroupIntegrationDB(t)
	defer cleanup()
	ctx:=context.Background()
	tenantID:="11111111-1111-4111-8111-111111111111"
	accountID:="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	t.Setenv("SESSION_ENCRYPTION_KEY",base64.StdEncoding.EncodeToString(make([]byte,32)))

	inviteCode:="AbCdEf123456"
	inviteHash:=sha256.Sum256([]byte(inviteCode))
	var publicID,inviteID int64
	if err:=db.QueryRowContext(ctx,`INSERT INTO public_group_links(link_type,public_url,status,last_check_error,check_attempts,next_check_at,last_checked_at,check_lease_until,check_lease_tenant_id,check_lease_account_id)
		VALUES('public','https://t.me/shared_group','unknown','temporary failure',4,NOW()+INTERVAL '1 hour',NOW(),NOW()+INTERVAL '5 minutes',$1,$2) RETURNING id`,tenantID,accountID).Scan(&publicID);err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,`INSERT INTO public_group_links(link_type,invite_hash,invite_ciphertext,status,last_check_error,check_attempts,next_check_at,last_checked_at,check_lease_until,check_lease_tenant_id,check_lease_account_id)
		VALUES('invite',$1,$2,'unknown','temporary failure',3,NOW()+INTERVAL '1 hour',NOW(),NOW()+INTERVAL '5 minutes',$3,$4) RETURNING id`,inviteHash[:],[]byte("encrypted"),tenantID,accountID).Scan(&inviteID);err!=nil{t.Fatal(err)}

	in:=Incoming{AccountID:accountID,ChatID:-100123,MessageID:77,ChatType:"group",Text:"https://t.me/shared_group and t.me/+AbCdEf123456"}
	if err:=withTenantTx(ctx,db,tenantID,func(tx *sql.Tx)error{return storeDiscoveredLinks(ctx,tx,tenantID,in)});err!=nil{t.Fatal(err)}

	for _,id:=range []int64{publicID,inviteID} {
		var status,lastError string
		var attempts int
		var nextCheck,lease sql.NullTime
		var leaseTenant,leaseAccount sql.NullString
		if err:=db.QueryRowContext(ctx,`SELECT status,last_check_error,check_attempts,next_check_at,check_lease_until,check_lease_tenant_id::text,check_lease_account_id::text FROM public_group_links WHERE id=$1`,id).Scan(&status,&lastError,&attempts,&nextCheck,&lease,&leaseTenant,&leaseAccount);err!=nil{t.Fatal(err)}
		if status!="unknown"||lastError!="temporary failure"||attempts<3||!nextCheck.Valid||!lease.Valid||!leaseTenant.Valid||!leaseAccount.Valid {
			t.Fatalf("rediscovery interrupted active check lease for link %d: status=%s error=%s attempts=%d next=%v lease=%v tenant=%v account=%v",id,status,lastError,attempts,nextCheck,lease,leaseTenant,leaseAccount)
		}
	}
	var sourceCount int
	if err:=db.QueryRowContext(ctx,`SELECT COUNT(*) FROM public_group_link_sources WHERE tenant_id=$1 AND telegram_account_id=$2`,tenantID,accountID).Scan(&sourceCount);err!=nil{t.Fatal(err)}
	if sourceCount!=2{t.Fatalf("rediscovered sources=%d, want 2",sourceCount)}

	// Once the old lease has expired, a fresh discovery should reset the stale
	// result and put the link back into the pending queue.
	if _,err:=db.ExecContext(ctx,`UPDATE public_group_links SET check_lease_until=NOW()-INTERVAL '1 second' WHERE id IN ($1,$2)`,publicID,inviteID);err!=nil{t.Fatal(err)}
	in.MessageID=78
	if err:=withTenantTx(ctx,db,tenantID,func(tx *sql.Tx)error{return storeDiscoveredLinks(ctx,tx,tenantID,in)});err!=nil{t.Fatal(err)}
	for _,id:=range []int64{publicID,inviteID} {
		var status string
		var lastError sql.NullString
		var attempts int
		var nextCheck,lease sql.NullTime
		if err:=db.QueryRowContext(ctx,`SELECT status,last_check_error,check_attempts,next_check_at,check_lease_until FROM public_group_links WHERE id=$1`,id).Scan(&status,&lastError,&attempts,&nextCheck,&lease);err!=nil{t.Fatal(err)}
		if status!="pending"||lastError.Valid||attempts!=0||nextCheck.Valid||lease.Valid {
			t.Fatalf("expired link lease was not requeued on rediscovery for %d: status=%s error=%v attempts=%d next=%v lease=%v",id,status,lastError,attempts,nextCheck,lease)
		}
	}
}


func TestJoinTaskPauseCancelAndRetryPreserveInflightRPCResults(t *testing.T) {
	db,cleanup:=openGroupIntegrationDB(t)
	defer cleanup()
	ctx:=context.Background()
	tenantID:="11111111-1111-4111-8111-111111111111"
	accountA:="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	accountB:="bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	taskPause:="33333333-3333-4333-8333-333333333333"
	taskCancel:="44444444-4444-4444-8444-444444444444"
	taskOther:="55555555-5555-4555-8555-555555555555"
	taskConcurrent:="66666666-6666-4666-8666-666666666666"
	if _,err:=db.ExecContext(ctx,`INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`,tenantID);err!=nil{t.Fatal(err)}
	for _,account:=range []string{accountA,accountB} {
		if _,err:=db.ExecContext(ctx,`INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext) VALUES($1,$2,'authorized',decode('01','hex'))`,tenantID,account);err!=nil{t.Fatal(err)}
	}
	for _,task:=range []struct{id,status string}{{taskPause,"queued"},{taskCancel,"queued"},{taskOther,"queued"},{taskConcurrent,"queued"}} {
		if _,err:=db.ExecContext(ctx,`INSERT INTO tenant_join_tasks(id,tenant_id,name,status) VALUES($1,$2,'integration task',$3)`,task.id,tenantID,task.status);err!=nil{t.Fatal(err)}
	}
	var pauseProcessing,pauseQueued,cancelProcessing,cancelQueued,otherProcessing,concurrentProcessing int64
	lease:=time.Now().UTC().Add(2*time.Minute)
	insertRow:=func(task,account,status string)(int64,error){
		var id int64
		err:=db.QueryRowContext(ctx,`INSERT INTO tenant_join_task_accounts(tenant_id,task_id,telegram_account_id,status,lease_until)
			VALUES($1,$2,$3,$4,CASE WHEN $4='processing' THEN $5::timestamptz ELSE NULL END) RETURNING id`,tenantID,task,account,status,lease).Scan(&id)
		return id,err
	}
	var err error
	if pauseProcessing,err=insertRow(taskPause,accountA,"processing");err!=nil{t.Fatal(err)}
	if pauseQueued,err=insertRow(taskPause,accountB,"queued");err!=nil{t.Fatal(err)}
	if cancelProcessing,err=insertRow(taskCancel,accountA,"processing");err!=nil{t.Fatal(err)}
	if cancelQueued,err=insertRow(taskCancel,accountB,"queued");err!=nil{t.Fatal(err)}
	if otherProcessing,err=insertRow(taskOther,accountA,"processing");err!=nil{t.Fatal(err)}
	if concurrentProcessing,err=insertRow(taskConcurrent,accountA,"processing");err!=nil{t.Fatal(err)}

	service:=New(db)
	if err:=service.PauseJoinTask(ctx,tenantID,taskPause);err!=nil{t.Fatal(err)}
	var status string
	var savedLease sql.NullTime
	if err:=db.QueryRowContext(ctx,`SELECT status,lease_until FROM tenant_join_task_accounts WHERE id=$1`,pauseProcessing).Scan(&status,&savedLease);err!=nil{t.Fatal(err)}
	if status!="processing"||!savedLease.Valid{t.Fatalf("pause revoked in-flight row: status=%s lease=%v",status,savedLease)}
	if err:=db.QueryRowContext(ctx,`SELECT status FROM tenant_join_task_accounts WHERE id=$1`,pauseQueued).Scan(&status);err!=nil{t.Fatal(err)}
	if status!="paused"{t.Fatalf("queued row after pause=%q",status)}
	job:=JoinTaskJob{TenantID:tenantID,TaskID:taskPause,AccountRowID:pauseProcessing,AccountID:accountA,LeaseUntil:lease}
	if err:=service.CompleteJoinTaskAccount(ctx,job,-100123,"not_required");err!=nil{t.Fatalf("persist in-flight successful join after pause: %v",err)}
	if err:=db.QueryRowContext(ctx,`SELECT status FROM tenant_join_task_accounts WHERE id=$1`,pauseProcessing).Scan(&status);err!=nil{t.Fatal(err)}
	if status!="joined"{t.Fatalf("in-flight successful join status=%q",status)}

	if err:=service.CancelJoinTask(ctx,tenantID,taskCancel);err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,`SELECT status,lease_until FROM tenant_join_task_accounts WHERE id=$1`,cancelProcessing).Scan(&status,&savedLease);err!=nil{t.Fatal(err)}
	if status!="processing"||!savedLease.Valid{t.Fatalf("cancel revoked in-flight row: status=%s lease=%v",status,savedLease)}
	if err:=db.QueryRowContext(ctx,`SELECT status FROM tenant_join_task_accounts WHERE id=$1`,cancelQueued).Scan(&status);err!=nil{t.Fatal(err)}
	if status!="cancelled"{t.Fatalf("queued row after cancel=%q",status)}
	cancelJob:=JoinTaskJob{TenantID:tenantID,TaskID:taskCancel,AccountRowID:cancelProcessing,AccountID:accountA,Attempts:1,LeaseUntil:lease}
	if err:=service.RescheduleJoinTaskRetry(ctx,cancelJob,"temporary Telegram error");err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,`SELECT status,lease_until FROM tenant_join_task_accounts WHERE id=$1`,cancelProcessing).Scan(&status,&savedLease);err!=nil{t.Fatal(err)}
	if status!="cancelled"||savedLease.Valid{t.Fatalf("cancelled in-flight error was requeued: status=%s lease=%v",status,savedLease)}

	// A FloodWait for one task must not revoke a second, still-running RPC
	// lease for the same account (for example, an expired-lease overlap).
	otherJob:=JoinTaskJob{TenantID:tenantID,TaskID:taskOther,AccountRowID:otherProcessing,AccountID:accountA,LeaseUntil:lease}
	if err:=service.RescheduleJoinTaskAccount(ctx,otherJob,90*time.Second,"FLOOD_WAIT_90");err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,`SELECT status,lease_until FROM tenant_join_task_accounts WHERE id=$1`,otherProcessing).Scan(&status,&savedLease);err!=nil{t.Fatal(err)}
	if status!="queued"||savedLease.Valid{t.Fatalf("current FloodWait row not rescheduled: status=%s lease=%v",status,savedLease)}
	if err:=db.QueryRowContext(ctx,`SELECT status,lease_until FROM tenant_join_task_accounts WHERE id=$1`,concurrentProcessing).Scan(&status,&savedLease);err!=nil{t.Fatal(err)}
	if status!="processing"||!savedLease.Valid{t.Fatalf("FloodWait revoked another active RPC lease: status=%s lease=%v",status,savedLease)}
}
