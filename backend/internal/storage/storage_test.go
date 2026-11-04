package storage

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestMigrateAppliesAllEmbeddedMigrationsToEmptySchema(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("TEST_DATABASE_URL is required in CI; refusing to skip migration integration test")
		}
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx := context.Background()
	adminConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	adminDB := stdlib.OpenDB(*adminConfig)
	defer adminDB.Close()

	schema := fmt.Sprintf("tgqf_migrate_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := adminDB.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("drop migration test schema: %v", err)
		}
	}()

	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(1)
	defer db.Close()

	store := &Store{DB: db}
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("apply embedded migrations to an empty schema: %v", err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count < 62 {
		t.Fatalf("applied migration count = %d, want at least 62", count)
	}

	var latest bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version='0062_keyword_voice_call_actions')").Scan(&latest); err != nil {
		t.Fatal(err)
	}
	if !latest {
		t.Fatal("latest voice-call migration was not applied")
	}
}

func TestSplitSQLStatementsPreservesDollarQuotedBodies(t *testing.T) {
	script := `-- semicolon in a comment ;
CREATE TABLE sample (value TEXT DEFAULT 'a;b');
CREATE FUNCTION sample_fn() RETURNS void LANGUAGE plpgsql AS $body$
BEGIN
  PERFORM 'inside;function';
  PERFORM 1;
END
$body$;
/* block ; comment */
INSERT INTO sample(value) VALUES ("not;identifier");`
	statements := splitSQLStatements(script)
	if len(statements) != 3 {
		t.Fatalf("got %d statements, want 3: %#v", len(statements), statements)
	}
	if !strings.Contains(statements[1], "PERFORM 'inside;function';") || !strings.Contains(statements[1], "END\n$body$") {
		t.Fatalf("function body was split: %s", statements[1])
	}
}

func TestDatabasePoolRecoversAfterBackendConnectionTermination(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("TEST_DATABASE_URL is required in CI; refusing to skip PostgreSQL recovery test")
		}
		t.Skip("TEST_DATABASE_URL is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer db.Close()
	admin := stdlib.OpenDB(*config)
	defer admin.Close()

	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var backendPID int
	if err := conn.QueryRowContext(ctx, "SELECT pg_backend_pid()").Scan(&backendPID); err != nil {
		conn.Close()
		t.Fatal(err)
	}
	if _, err := admin.ExecContext(ctx, "SELECT pg_terminate_backend($1)", backendPID); err != nil {
		conn.Close()
		t.Fatalf("terminate pooled PostgreSQL connection: %v", err)
	}
	_ = conn.Close()

	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		pingCtx, pingCancel := context.WithTimeout(ctx, 3*time.Second)
		lastErr = db.PingContext(pingCtx)
		pingCancel()
		if lastErr == nil {
			var one int
			if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
				t.Fatalf("query after PostgreSQL reconnect: %v", err)
			}
			if one != 1 {
				t.Fatalf("query after reconnect returned %d, want 1", one)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("database pool did not reconnect after backend termination: %v", lastErr)
}
