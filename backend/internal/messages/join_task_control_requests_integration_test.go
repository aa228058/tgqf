package messages

import (
	"context"
	"database/sql"
	"testing"
	"strconv"
	"time"
)

func TestJoinTaskControlRequestsDoNotRevokeInflightLeases(t *testing.T) {
	db,cleanup:=openGroupIntegrationDB(t)
	defer cleanup()
	ctx:=context.Background()
	tenantID:="11111111-1111-4111-8111-111111111111"
	accountA:="aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	accountB:="bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	if _,err:=db.ExecContext(ctx,`INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`,tenantID);err!=nil{t.Fatal(err)}
	for _,account:=range []string{accountA,accountB}{
		if _,err:=db.ExecContext(ctx,`INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext) VALUES($1,$2,'authorized',decode('01','hex'))`,tenantID,account);err!=nil{t.Fatal(err)}
	}
	insertTask:=func(id,status string){t.Helper();if _,err:=db.ExecContext(ctx,`INSERT INTO tenant_join_tasks(id,tenant_id,name,status) VALUES($1,$2,'task control',$3)`,id,tenantID,status);err!=nil{t.Fatal(err)}}
	lease:=time.Now().UTC().Truncate(time.Microsecond).Add(2*time.Minute)
	insertRow:=func(task,account,status string)int64{
		t.Helper()
		var leaseValue any
		if status=="processing"{leaseValue=lease}
		var id int64
		if err:=db.QueryRowContext(ctx,`INSERT INTO tenant_join_task_accounts(tenant_id,task_id,telegram_account_id,status,lease_until)
			VALUES($1,$2,$3,$4,$5) RETURNING id`,tenantID,task,account,status,leaseValue).Scan(&id);err!=nil{t.Fatal(err)}
		return id
	}
	service:=New(db)
	childPause:="33333333-3333-4333-8333-333333333333"
	insertTask(childPause,"queued")
	processingID:=insertRow(childPause,accountA,"processing")
	_ = insertRow(childPause,accountB,"queued")
	if err:=service.ChangeUnifiedTaskStatus(ctx,tenantID,"group_join",strconv.FormatInt(processingID,10),"pause");err!=nil{t.Fatal(err)}
	var status string
	var pauseRequested,cancelRequested bool
	var savedLease sql.NullTime
	if err:=db.QueryRowContext(ctx,`SELECT status,pause_requested,cancel_requested,lease_until FROM tenant_join_task_accounts WHERE id=$1`,processingID).Scan(&status,&pauseRequested,&cancelRequested,&savedLease);err!=nil{t.Fatal(err)}
	if status!="processing"||!pauseRequested||cancelRequested||!savedLease.Valid{t.Fatalf("pause revoked active lease or lost request: %s %v %v %v",status,pauseRequested,cancelRequested,savedLease)}
	job:=JoinTaskJob{TenantID:tenantID,TaskID:childPause,AccountRowID:processingID,AccountID:accountA,Attempts:1,LeaseUntil:lease}
	if err:=service.RescheduleJoinTaskRetry(ctx,job,"temporary Telegram error");err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,`SELECT status,pause_requested,lease_until FROM tenant_join_task_accounts WHERE id=$1`,processingID).Scan(&status,&pauseRequested,&savedLease);err!=nil{t.Fatal(err)}
	if status!="paused"||pauseRequested||savedLease.Valid{t.Fatalf("pause request was not consumed after RPC: %s %v %v",status,pauseRequested,savedLease)}

	childCancel:="44444444-4444-4444-8444-444444444444"
	insertTask(childCancel,"queued")
	cancelID:=insertRow(childCancel,accountA,"processing")
	if err:=service.ChangeUnifiedTaskStatus(ctx,tenantID,"group_join",strconv.FormatInt(cancelID,10),"cancel");err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,`SELECT status,cancel_requested,lease_until FROM tenant_join_task_accounts WHERE id=$1`,cancelID).Scan(&status,&cancelRequested,&savedLease);err!=nil{t.Fatal(err)}
	if status!="processing"||!cancelRequested||!savedLease.Valid{t.Fatalf("cancel revoked active lease: %s %v %v",status,cancelRequested,savedLease)}
	cancelJob:=JoinTaskJob{TenantID:tenantID,TaskID:childCancel,AccountRowID:cancelID,AccountID:accountA,Attempts:1,LeaseUntil:lease}
	if err:=service.RescheduleJoinTaskRetry(ctx,cancelJob,"temporary Telegram error");err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,`SELECT status,cancel_requested,lease_until FROM tenant_join_task_accounts WHERE id=$1`,cancelID).Scan(&status,&cancelRequested,&savedLease);err!=nil{t.Fatal(err)}
	if status!="cancelled"||cancelRequested||savedLease.Valid{t.Fatalf("cancel request was requeued: %s %v %v",status,cancelRequested,savedLease)}

	parentPause:="55555555-5555-4555-8555-555555555555"
	insertTask(parentPause,"queued")
	parentProcessing:=insertRow(parentPause,accountA,"processing")
	parentQueued:=insertRow(parentPause,accountB,"queued")
	if err:=service.PauseJoinTask(ctx,tenantID,parentPause);err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,`SELECT status,pause_requested,lease_until FROM tenant_join_task_accounts WHERE id=$1`,parentProcessing).Scan(&status,&pauseRequested,&savedLease);err!=nil{t.Fatal(err)}
	if status!="processing"||!pauseRequested||!savedLease.Valid{t.Fatalf("parent pause revoked active lease: %s %v %v",status,pauseRequested,savedLease)}
	if err:=db.QueryRowContext(ctx,`SELECT status FROM tenant_join_task_accounts WHERE id=$1`,parentQueued).Scan(&status);err!=nil{t.Fatal(err)}
	if status!="paused"{t.Fatalf("queued child after parent pause=%s",status)}
	parentJob:=JoinTaskJob{TenantID:tenantID,TaskID:parentPause,AccountRowID:parentProcessing,AccountID:accountA,LeaseUntil:lease}
	if err:=service.CompleteJoinTaskAccount(ctx,parentJob,-100123,"not_required");err!=nil{t.Fatal(err)}
	if err:=db.QueryRowContext(ctx,`SELECT status FROM tenant_join_tasks WHERE id=$1`,parentPause).Scan(&status);err!=nil{t.Fatal(err)}
	if status!="paused"{t.Fatalf("parent task status after in-flight success and paused sibling=%s",status)}
}
