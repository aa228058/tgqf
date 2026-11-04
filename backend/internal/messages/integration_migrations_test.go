package messages

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aa228058/tgqf/backend/internal/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// openMigratedIntegrationDB gives tests an isolated schema built from the exact
// production migration set. Use it for tests that exercise cross-module writes
// so test-only hand-maintained DDL cannot silently drift behind migrations.
func openMigratedIntegrationDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	adminDB := stdlib.OpenDB(*adminConfig)
	schema := fmt.Sprintf("tgqf_migrated_%d", time.Now().UnixNano())
	if _, err := adminDB.Exec("CREATE SCHEMA " + schema); err != nil {
		_ = adminDB.Close()
		t.Fatal(err)
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		_, _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE")
		_ = adminDB.Close()
		t.Fatal(err)
	}
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(8)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	store := &storage.Store{DB: db}
	if err := store.Migrate(ctx); err != nil {
		_ = db.Close()
		_, _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE")
		_ = adminDB.Close()
		t.Fatalf("apply production migrations to isolated integration schema: %v", err)
	}
	cleanup := func() {
		_ = db.Close()
		_, _ = adminDB.Exec("DROP SCHEMA " + schema + " CASCADE")
		_ = adminDB.Close()
	}
	return db, cleanup
}
