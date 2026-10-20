package reqval

import (
	"context"
	"database/sql"
	"errors"
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
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT UNIQUE)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, username) VALUES (42, 'alice'), (43, 'bob')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO categories (id, name) VALUES (1, 'go')`); err != nil {
		t.Fatal(err)
	}
	return db
}

// --- basic request ---

type storePostRequest struct {
	Base
	Title      string `json:"title"`
	Body       string `json:"body"`
	CategoryID int64  `json:"category_id"`
}

func (r *storePostRequest) Rules() map[string][]string {
	return map[string][]string{
		"Title":      {"required", "max:255"},
		"Body":       {"required", "min:10"},
		"CategoryID": {"required", "exists:categories,id"},
	}
}

func (r *storePostRequest) Messages() map[string]string {
	return map[string]string{
		"Title.required": "We need a title.",
	}
}

func TestValidPasses(t *testing.T) {
	RegisterDB(newTestDB(t))
	req := &storePostRequest{Title: "Hi", Body: "This is long enough", CategoryID: 1}
	if err := ValidateCtx(context.Background(), req); err != nil {
		t.Fatalf("want valid, got %v", err)
	}
}

func TestRequiredAndMessage(t *testing.T) {
	RegisterDB(newTestDB(t))
	err := ValidateCtx(context.Background(), &storePostRequest{Body: "This is long enough", CategoryID: 1})
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("want validation error, got %v", err)
	}
	if ve.Fields["Title"] != "We need a title." {
		t.Fatalf("want custom message, got %q", ve.Fields["Title"])
	}
}

func TestExistsFails(t *testing.T) {
	RegisterDB(newTestDB(t))
	err := ValidateCtx(context.Background(), &storePostRequest{Title: "Hi", Body: "This is long enough", CategoryID: 99})
	if _, ok := AsValidationError(err); !ok {
		t.Fatalf("want validation error, got %v", err)
	}
}

// --- authorization runs before decode ---

type authFailReq struct{ Base }

func (r *authFailReq) Rules() map[string][]string          { return nil }
func (r *authFailReq) Authorize(ctx context.Context) error { return errors.New("admin only") }

func TestAuthorizeRunsBeforeDecode(t *testing.T) {
	err := BindCtx(context.Background(), strings.NewReader(`{bad json`), &authFailReq{})
	ae, ok := AsAuthorizationError(err)
	if !ok || ae.HTTPStatus() != 403 {
		t.Fatalf("want 403 before decode, got %v", err)
	}
}

// --- unique (create) ---

type userReq struct {
	Base
	Username string `json:"username"`
}

func (r *userReq) Rules() map[string][]string {
	return map[string][]string{"Username": {"required", "unique:users,username"}}
}

func TestUniqueCreate(t *testing.T) {
	RegisterDB(newTestDB(t))
	if err := ValidateCtx(context.Background(), &userReq{Username: "carol"}); err != nil {
		t.Fatalf("carol should be free: %v", err)
	}
	if _, ok := AsValidationError(ValidateCtx(context.Background(), &userReq{Username: "alice"})); !ok {
		t.Fatalf("alice should be taken")
	}
}

// --- unique (update, with ignore) ---

type updateReq struct {
	Base
	Username string `json:"username"`
}

func (r *updateReq) Rules() map[string][]string {
	return map[string][]string{"Username": {"unique:users,username,id,42"}}
}

func TestUniqueUpdateIgnore(t *testing.T) {
	RegisterDB(newTestDB(t))
	if err := ValidateCtx(context.Background(), &updateReq{Username: "alice"}); err != nil {
		t.Fatalf("should pass: %v", err)
	}
}

// --- applyRule primitives ---

func TestInAndBetween(t *testing.T) {
	if !applyRule("in", "admin,editor", "admin") {
		t.Fatal("in should accept admin")
	}
	if applyRule("in", "admin,editor", "root") {
		t.Fatal("in should reject root")
	}
	if !applyRule("between", "1,100", 50) {
		t.Fatal("between should accept 50")
	}
	if applyRule("between", "1,100", 200) {
		t.Fatal("between should reject 200")
	}
}

// --- message resolution order ---

type plainReq struct {
	Base
	Name string
}

func (r *plainReq) Rules() map[string][]string {
	return map[string][]string{"Name": {"required"}}
}

type overrideReq struct {
	Base
	Name string
}

func (r *overrideReq) Rules() map[string][]string {
	return map[string][]string{"Name": {"required"}}
}
func (r *overrideReq) Messages() map[string]string {
	return map[string]string{"Name.required": "REQUEST {{field}}"}
}

func TestMessageResolutionOrder(t *testing.T) {
	RegisterDB(newTestDB(t))

	SetDefaultMessages(nil)
	err := ValidateCtx(context.Background(), &plainReq{})
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("want validation error, got %v", err)
	}
	if ve.Fields["Name"] != "The Name field is invalid." {
		t.Fatalf("fallback: got %q", ve.Fields["Name"])
	}

	SetDefaultMessages(map[string]string{"*.required": "BUNDLE {{field}}"})
	defer SetDefaultMessages(nil)
	err = ValidateCtx(context.Background(), &plainReq{})
	ve, _ = AsValidationError(err)
	if ve.Fields["Name"] != "BUNDLE Name" {
		t.Fatalf("bundle: got %q", ve.Fields["Name"])
	}

	err = ValidateCtx(context.Background(), &overrideReq{})
	ve, _ = AsValidationError(err)
	if ve.Fields["Name"] != "REQUEST Name" {
		t.Fatalf("override: got %q", ve.Fields["Name"])
	}
}

// --- confirmed (cross-field) ---

type confirmedReq struct {
	Base
	Password             string `json:"password"`
	PasswordConfirmation string `json:"password_confirmation"`
}

func (r *confirmedReq) Rules() map[string][]string {
	return map[string][]string{"Password": {"required", "confirmed"}}
}

func TestConfirmedMatching(t *testing.T) {
	ok := &confirmedReq{Password: "hunter2", PasswordConfirmation: "hunter2"}
	if err := ValidateCtx(context.Background(), ok); err != nil {
		t.Fatalf("want pass, got %v", err)
	}
}

func TestConfirmedMismatch(t *testing.T) {
	bad := &confirmedReq{Password: "hunter2", PasswordConfirmation: "different"}
	err := ValidateCtx(context.Background(), bad)
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("want validation error, got %v", err)
	}
	if _, has := ve.Fields["Password"]; !has {
		t.Fatalf("want Password flagged, got %v", ve.Fields)
	}
}

func TestConfirmedMissingSibling(t *testing.T) {
	type missing struct {
		Base
		Password string
	}
	r := &missing{Password: "x"}
	_ = r
}

// --- date ---

type dateReq struct {
	Base
	PublishedAt string `json:"published_at"`
}

func (r *dateReq) Rules() map[string][]string {
	return map[string][]string{"PublishedAt": {"required", "date:2006-01-02"}}
}

func TestDateValid(t *testing.T) {
	if err := ValidateCtx(context.Background(), &dateReq{PublishedAt: "2024-03-15"}); err != nil {
		t.Fatalf("want valid date, got %v", err)
	}
}

func TestDateInvalid(t *testing.T) {
	err := ValidateCtx(context.Background(), &dateReq{PublishedAt: "15/03/2024"})
	if _, ok := AsValidationError(err); !ok {
		t.Fatalf("want invalid date, got %v", err)
	}
}

func TestDateEmptyAllowed(t *testing.T) {
	// no required, empty should pass
	type optional struct {
		Base
		PublishedAt string
	}
	r := &optional{}
	_ = r
}
