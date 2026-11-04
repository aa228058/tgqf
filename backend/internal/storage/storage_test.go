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
