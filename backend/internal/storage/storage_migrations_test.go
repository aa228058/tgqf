package storage

import (
	"reflect"
	"testing"
)

func TestSplitSQLStatementsPreservesQuotedBodies(t *testing.T) {
	script := `-- migration preamble; ignored
CREATE TABLE sample (value TEXT DEFAULT 'semi;colon', "odd;name" TEXT);
CREATE FUNCTION demo() RETURNS void LANGUAGE plpgsql AS $body$
BEGIN
  PERFORM 'inside;function';
  /* nested comment; /* still comment; */ done */
  PERFORM 1;
END;
$body$;
-- trailing comment; ignored
INSERT INTO sample(value) VALUES ('it''s; fine');`
	want := []string{
		`-- migration preamble; ignored
CREATE TABLE sample (value TEXT DEFAULT 'semi;colon', "odd;name" TEXT)`,
		`CREATE FUNCTION demo() RETURNS void LANGUAGE plpgsql AS $body$
BEGIN
  PERFORM 'inside;function';
  /* nested comment; /* still comment; */ done */
  PERFORM 1;
END;
$body$`,
		`-- trailing comment; ignored
INSERT INTO sample(value) VALUES ('it''s; fine')`,
	}
	if got := splitSQLStatements(script); !reflect.DeepEqual(got, want) {
		t.Fatalf("splitSQLStatements() mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestSplitSQLStatementsSupportsUntaggedDollarQuotes(t *testing.T) {
	script := `DO $$ BEGIN RAISE NOTICE 'one;two'; END $$; SELECT 1;`
	want := []string{`DO $$ BEGIN RAISE NOTICE 'one;two'; END $$`, `SELECT 1`}
	if got := splitSQLStatements(script); !reflect.DeepEqual(got, want) {
		t.Fatalf("splitSQLStatements() = %#v, want %#v", got, want)
	}
}
