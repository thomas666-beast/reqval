package reqval

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	mustExec(t, db, `CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT UNIQUE, email TEXT UNIQUE)`)
	mustExec(t, db, `INSERT INTO users (id, username, email) VALUES (42, 'alice', 'alice@example.com'), (43, 'bob', 'bob@example.com')`)
	mustExec(t, db, `CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT)`)
	mustExec(t, db, `INSERT INTO categories (id, name) VALUES (1, 'go'), (2, 'php')`)
	return db
}

func mustExec(t *testing.T, db *sql.DB, q string) {
	t.Helper()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

type basicReq struct {
	Base
	Title string `validate:"required,max=5"`
	Body  string `validate:"required,min=3"`
}

func TestBasicRules(t *testing.T) {
	cases := []struct {
		name      string
		req       basicReq
		wantValid bool
		wantField string
	}{
		{"valid", basicReq{Title: "hi", Body: "hello"}, true, ""},
		{"missing title", basicReq{Body: "hello"}, false, "Title"},
		{"title too long", basicReq{Title: "toolong", Body: "hello"}, false, "Title"},
		{"body too short", basicReq{Title: "hi", Body: "x"}, false, "Body"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(&tc.req)
			if tc.wantValid {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			ve, ok := AsValidationError(err)
			if !ok {
				t.Fatalf("want validation error, got %v", err)
			}
			if _, ok := ve.Fields[tc.wantField]; !ok {
				t.Fatalf("want field %q, got %v", tc.wantField, ve.Fields)
			}
			if ve.HTTPStatus() != 422 {
				t.Fatalf("want 422, got %d", ve.HTTPStatus())
			}
		})
	}
}

type msgReq struct {
	Base
	Title string `validate:"required"`
}

func TestMessages(t *testing.T) {
	defer RegisterMessage("required", "The {{field}} field is required.")

	RegisterMessage("required", "CUSTOM {{field}}")
	ve, _ := AsValidationError(Validate(&msgReq{}))
	if got := ve.Fields["Title"]; got != "CUSTOM Title" {
		t.Fatalf("tag override: got %q", got)
	}

	RegisterFieldMessage("msgReq", "Title", "FIELD {{field}}")
	defer func() {
		msgMu.Lock()
		delete(fieldMsgs["msgReq"], "Title")
		msgMu.Unlock()
	}()
	ve, _ = AsValidationError(Validate(&msgReq{}))
	if got := ve.Fields["Title"]; got != "FIELD Title" {
		t.Fatalf("field override: got %q", got)
	}
}

type createUserReq struct {
	Base
	Username string `validate:"required,unique=users:username"`
}

func TestUniqueCreate(t *testing.T) {
	RegisterDB(newTestDB(t))

	if err := Validate(&createUserReq{Username: "carol"}); err != nil {
		t.Fatalf("carol should be free: %v", err)
	}

	err := Validate(&createUserReq{Username: "alice"})
	ve, ok := AsValidationError(err)
	if !ok || ve.Fields["Username"] == "" {
		t.Fatalf("alice should fail unique: %v", err)
	}
}

type updateUserReq struct {
	Base
	Username string `validate:"required,unique=users:username:id:42"`
}

func TestUniqueUpdateIgnoreSelf(t *testing.T) {
	RegisterDB(newTestDB(t))

	if err := Validate(&updateUserReq{Username: "alice"}); err != nil {
		t.Fatalf("keeping own username should pass: %v", err)
	}

	err := Validate(&updateUserReq{Username: "bob"})
	if _, ok := AsValidationError(err); !ok {
		t.Fatalf("taking bob's name should fail, got %v", err)
	}
}

type storePostReq struct {
	Base
	Title      string `validate:"required,max=255"`
	CategoryID int64  `validate:"required,exists=categories:id"`
}

func TestExists(t *testing.T) {
	RegisterDB(newTestDB(t))

	if err := Validate(&storePostReq{Title: "x", CategoryID: 1}); err != nil {
		t.Fatalf("category 1 exists: %v", err)
	}

	err := Validate(&storePostReq{Title: "x", CategoryID: 99})
	if _, ok := AsValidationError(err); !ok {
		t.Fatalf("category 99 should fail exists, got %v", err)
	}

	err = Validate(&storePostReq{Title: "x"})
	if _, ok := AsValidationError(err); !ok {
		t.Fatalf("required should fire on 0, got %v", err)
	}
}

type authReq struct {
	Base
	Title string `validate:"required"`
}

func (a *authReq) Authorize(ctx context.Context) error {
	if v, _ := ctx.Value("role").(string); v != "admin" {
		return fmt.Errorf("admin only")
	}
	return nil
}

func TestAuthorize(t *testing.T) {
	ctx := context.WithValue(context.Background(), "role", "editor")
	err := BindCtx(ctx, strings.NewReader(`{"Title":"x"}`), &authReq{})
	ae, ok := AsAuthorizationError(err)
	if !ok || ae.HTTPStatus() != 403 {
		t.Fatalf("want 403 authz error, got %v", err)
	}

	ctx = context.WithValue(context.Background(), "role", "admin")
	if err := BindCtx(ctx, strings.NewReader(`{"Title":"x"}`), &authReq{}); err != nil {
		t.Fatalf("admin should pass: %v", err)
	}
}

type injectReq struct {
	Base
	Username string `validate:"unique=users;DROP TABLE users:username"`
}

func TestUniqueInjectionSafe(t *testing.T) {
	db := newTestDB(t)
	RegisterDB(db)

	_ = Validate(&injectReq{Username: "bob"})

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("users table gone: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
}

type dynamicUserReq struct {
	Base
	Username string `validate:"required,unique=users:username:id"`
}

func TestUniqueDynamicIgnoreCreate(t *testing.T) {
	RegisterDB(newTestDB(t))
	if err := ValidateCtx(context.Background(), &dynamicUserReq{Username: "carol"}); err != nil {
		t.Fatalf("carol should be free: %v", err)
	}
	err := ValidateCtx(context.Background(), &dynamicUserReq{Username: "alice"})
	if _, ok := AsValidationError(err); !ok {
		t.Fatalf("alice should be taken: %v", err)
	}
}

func TestUniqueDynamicIgnoreUpdate(t *testing.T) {
	RegisterDB(newTestDB(t))
	ctx := WithIgnoreID(context.Background(), int64(42))

	if err := ValidateCtx(ctx, &dynamicUserReq{Username: "alice"}); err != nil {
		t.Fatalf("keeping own username should pass: %v", err)
	}
	err := ValidateCtx(ctx, &dynamicUserReq{Username: "bob"})
	if _, ok := AsValidationError(err); !ok {
		t.Fatalf("bob is taken by 43, should fail: %v", err)
	}
}

type rulesReq struct {
	Base
	Email                string `validate:"required,email"`
	Password             string `validate:"required,min=8,confirmed"`
	PasswordConfirmation string
	Role                 string `validate:"required,in=admin editor"`
	Score                int    `validate:"between=1 100"`
	Code                 string `validate:"omitempty,regexp=^[A-Z]{3}$"`
}

func TestExtraRules(t *testing.T) {
	valid := rulesReq{
		Email:                "a@b.com",
		Password:             "longenough",
		PasswordConfirmation: "longenough",
		Role:                 "admin",
		Score:                50,
		Code:                 "ABC",
	}
	if err := Validate(&valid); err != nil {
		t.Fatalf("valid case failed: %v", err)
	}

	cases := map[string]rulesReq{
		"bad email":   {Email: "no", Password: "longenough", PasswordConfirmation: "longenough", Role: "admin", Score: 50},
		"mismatch":    {Email: "a@b.com", Password: "longenough", PasswordConfirmation: "different", Role: "admin", Score: 50},
		"bad role":    {Email: "a@b.com", Password: "longenough", PasswordConfirmation: "longenough", Role: "root", Score: 50},
		"score range": {Email: "a@b.com", Password: "longenough", PasswordConfirmation: "longenough", Role: "admin", Score: 200},
		"bad regex":   {Email: "a@b.com", Password: "longenough", PasswordConfirmation: "longenough", Role: "admin", Score: 50, Code: "abc"},
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			err := Validate(&req)
			if _, ok := AsValidationError(err); !ok {
				t.Fatalf("want validation error, got %v", err)
			}
		})
	}
}
