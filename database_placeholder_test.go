package reqval

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// stubQuerier records the last query and args, then returns a *sql.Row from
// a real (but unopened/closed) sql.DB so that Scan fails cleanly with an
// error instead of panicking on a zero *sql.Row.
type stubQuerier struct {
	lastQuery string
	lastArgs  []any
	row       *sql.Row
}

func newStubQuerier(t *testing.T) *stubQuerier {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// Close immediately: any Scan on a Row from this DB returns
	// "sql: database is closed" — an error, not a panic.
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}
	return &stubQuerier{row: db.QueryRow("SELECT 1")}
}

func (s *stubQuerier) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	s.lastQuery = query
	s.lastArgs = args
	return s.row
}

func TestUniquePlaceholderSQLiteStyle(t *testing.T) {
	stub := newStubQuerier(t)
	RegisterDB(stub)
	t.Cleanup(func() { RegisterDB(nil) })

	_ = checkUnique(context.Background(), "users,email", "alice@example.com")

	if stub.lastQuery == "" {
		t.Fatal("no query captured")
	}
	if !strings.Contains(stub.lastQuery, "?") {
		t.Fatalf("want ? placeholder, got %q", stub.lastQuery)
	}
	if strings.Contains(stub.lastQuery, "$1") {
		t.Fatalf("did not want $1 placeholder, got %q", stub.lastQuery)
	}
}

func TestUniquePlaceholderPostgresStyle(t *testing.T) {
	stub := newStubQuerier(t)
	RegisterDBWithPlaceholder(stub, "$")
	t.Cleanup(func() { RegisterDB(nil) })

	_ = checkUnique(context.Background(), "users,email", "alice@example.com")

	if stub.lastQuery == "" {
		t.Fatal("no query captured")
	}
	if !strings.Contains(stub.lastQuery, "$1") {
		t.Fatalf("want $1 placeholder, got %q", stub.lastQuery)
	}
	if strings.Contains(stub.lastQuery, "?") {
		t.Fatalf("did not want ? placeholder, got %q", stub.lastQuery)
	}
}

func TestUniqueUpdatePlaceholderPostgresStyle(t *testing.T) {
	stub := newStubQuerier(t)
	RegisterDBWithPlaceholder(stub, "$")
	t.Cleanup(func() { RegisterDB(nil) })

	_ = checkUnique(context.Background(), "users,email,id,42", "alice@example.com")

	if stub.lastQuery == "" {
		t.Fatal("no query captured")
	}
	if !strings.Contains(stub.lastQuery, "$1") || !strings.Contains(stub.lastQuery, "$2") {
		t.Fatalf("want $1 and $2 placeholders, got %q", stub.lastQuery)
	}
	if strings.Contains(stub.lastQuery, "?") {
		t.Fatalf("did not want ? placeholder, got %q", stub.lastQuery)
	}
}

func TestExistsPlaceholderPostgresStyle(t *testing.T) {
	stub := newStubQuerier(t)
	RegisterDBWithPlaceholder(stub, "$")
	t.Cleanup(func() { RegisterDB(nil) })

	_ = checkExists(context.Background(), "categories,id", int64(1))

	if stub.lastQuery == "" {
		t.Fatal("no query captured")
	}
	if !strings.Contains(stub.lastQuery, "$1") {
		t.Fatalf("want $1 placeholder, got %q", stub.lastQuery)
	}
	if strings.Contains(stub.lastQuery, "?") {
		t.Fatalf("did not want ? placeholder, got %q", stub.lastQuery)
	}
}

func TestRegisterDBStyleUnknownFallsBack(t *testing.T) {
	stub := newStubQuerier(t)
	RegisterDBWithPlaceholder(stub, "postgres") // not "$"
	t.Cleanup(func() { RegisterDB(nil) })

	_ = checkExists(context.Background(), "categories,id", int64(1))

	if stub.lastQuery == "" {
		t.Fatal("no query captured")
	}
	if !strings.Contains(stub.lastQuery, "?") {
		t.Fatalf("want fallback ? placeholder, got %q", stub.lastQuery)
	}
}
