package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestRewritePostgresDatabaseInURLDSN(t *testing.T) {
	got, ok := rewritePostgresDatabaseInDSN("postgres://user:pass@127.0.0.1:5432/twilight?sslmode=disable", "postgres")
	if !ok {
		t.Fatal("expected URL DSN to be rewritten")
	}
	if got != "postgres://user:pass@127.0.0.1:5432/postgres?sslmode=disable" {
		t.Fatalf("unexpected rewritten DSN: %s", got)
	}
}

func TestRewritePostgresDatabaseInKeywordDSN(t *testing.T) {
	got, ok := rewritePostgresDatabaseInDSN("host=127.0.0.1 port=5432 user=twilight password=secret dbname=twilight sslmode=disable", "template1")
	if !ok {
		t.Fatal("expected keyword DSN to be rewritten")
	}
	if !strings.Contains(got, "://twilight:secret@127.0.0.1:5432/template1") || !strings.Contains(got, "sslmode=disable") {
		t.Fatalf("unexpected rewritten keyword DSN: %s", got)
	}
}

func TestPostgresErrorClassifiersAndIdentifierQuoting(t *testing.T) {
	if !isUndefinedDatabaseError(&pgconn.PgError{Code: "3D000"}) {
		t.Fatal("undefined database error was not detected")
	}
	if !isDuplicateDatabaseError(errors.New("ERROR: duplicate_database (SQLSTATE 42P04)")) {
		t.Fatal("duplicate database error was not detected")
	}
	if got := quotePostgresIdentifier(`twilight"prod`); got != `"twilight""prod"` {
		t.Fatalf("identifier was not quoted safely: %s", got)
	}
}
