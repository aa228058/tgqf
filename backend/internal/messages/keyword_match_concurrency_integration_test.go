package messages

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestConcurrentIncomingUpdateCreatesOneKeywordMatchAndAction(t *testing.T) {
	db, cleanup := openGroupIntegrationDB(t)
	defer cleanup()
	ctx := context.Background()
	tenantID := "12121212-1212-4212-8212-121212121212"
	accountID := "abababab-abab-4bab-8bab-abababababab"
	if _, err := db.ExecContext(ctx, `INSERT INTO tenants(id,status,expires_at) VALUES($1,'active',NOW()+INTERVAL '30 days')`, tenantID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO telegram_accounts(tenant_id,id,status,session_ciphertext) VALUES($1,$2,'authorized',decode('01','hex'))`, tenantID, accountID); err != nil {
		t.Fatal(err)
	}
	if _, err := New(db).CreateKeywordRule(ctx, tenantID, KeywordRule{
		Name: "concurrent match", MatchMode: "any", Keywords: []string{"hello"},
		ChatTypes: []string{"private"}, ActionType: "text", ReplyText: "Thanks for your message",
		Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	incoming := Incoming{
		AccountID: accountID, ChatID: 12345, MessageID: 987,
		ChatType: "private", Text: "hello there", MessageDate: time.Now().UTC(),
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := New(db).Ingest(ctx, tenantID, incoming)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var matches, actions, dedup int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenant_keyword_matches WHERE tenant_id=$1`, tenantID).Scan(&matches); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM keyword_action_jobs WHERE tenant_id=$1`, tenantID).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM telegram_incoming_update_dedup WHERE tenant_id=$1`, tenantID).Scan(&dedup); err != nil {
		t.Fatal(err)
	}
	if matches != 1 || actions != 1 || dedup != 1 {
		t.Fatalf("concurrent duplicate ingest produced matches/actions/dedup=%d/%d/%d, want 1/1/1", matches, actions, dedup)
	}
}
