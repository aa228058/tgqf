package messages

import (
	"context"
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
}
