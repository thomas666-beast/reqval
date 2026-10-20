package reqval

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	return map[string]string{"Title.required": "We need a title."}
}

func TestValidPasses(t *testing.T) {
	RegisterDB(newTestDB(t))
	if err := ValidateCtx(context.Background(), &storePostRequest{Title: "Hi", Body: "This is long enough", CategoryID: 1}); err != nil {
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

type confirmedReq struct {
	Base
	Password             string
	PasswordConfirmation string
}

func (r *confirmedReq) Rules() map[string][]string {
	return map[string][]string{"Password": {"required", "confirmed"}}
}

func TestConfirmedMatching(t *testing.T) {
	if err := ValidateCtx(context.Background(), &confirmedReq{Password: "hunter2", PasswordConfirmation: "hunter2"}); err != nil {
		t.Fatalf("want pass, got %v", err)
	}
}

func TestConfirmedMismatch(t *testing.T) {
	err := ValidateCtx(context.Background(), &confirmedReq{Password: "hunter2", PasswordConfirmation: "different"})
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("want validation error, got %v", err)
	}
	if _, has := ve.Fields["Password"]; !has {
		t.Fatalf("want Password flagged, got %v", ve.Fields)
	}
}

type dateReq struct {
	Base
	PublishedAt string
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

type uploadReq struct {
	Base
	Description string `form:"description"`
}

func (r *uploadReq) Rules() map[string][]string {
	return map[string][]string{"Description": {"required", "max:200"}}
}

func (r *uploadReq) FileRules() []FileRule {
	return []FileRule{
		{Field: "avatar", MaxBytes: 2 << 20, MIMEs: []string{"image/png", "image/jpeg"}},
	}
}

func TestUploadValidPNG(t *testing.T) {
	body, ctype := buildMultipart(t, "my avatar", "avatar.png", []byte{
		0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A,
		0, 0, 0, 0, 0, 0, 0, 0,
	})
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ctype)

	var dst uploadReq
	if err := BindMultipartCtx(context.Background(), req, &dst); err != nil {
		t.Fatalf("want valid upload, got %v", err)
	}
	if dst.Description != "my avatar" {
		t.Fatalf("want description decoded, got %q", dst.Description)
	}
}

func TestUploadWrongMIME(t *testing.T) {
	body, ctype := buildMultipart(t, "my avatar", "avatar.txt", []byte("plain text"))
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ctype)

	var dst uploadReq
	err := BindMultipartCtx(context.Background(), req, &dst)
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("want validation error, got %v", err)
	}
	if _, has := ve.Fields["avatar"]; !has {
		t.Fatalf("want avatar flagged, got %v", ve.Fields)
	}
}

func TestUploadMissingFile(t *testing.T) {
	body, ctype := buildMultipart(t, "my avatar", "", nil)
	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", ctype)

	var dst uploadReq
	err := BindMultipartCtx(context.Background(), req, &dst)
	if _, ok := AsValidationError(err); !ok {
		t.Fatalf("want validation error for missing file, got %v", err)
	}
}

func buildMultipart(t *testing.T, desc, filename string, content []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if desc != "" {
		_ = w.WriteField("description", desc)
	}
	if filename != "" {
		fw, err := w.CreateFormFile("avatar", filename)
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(content)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

type Item struct {
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

type orderReq struct {
	Base
	Items []Item `json:"items"`
}

func (r *orderReq) Rules() map[string][]string {
	return map[string][]string{
		"Items.*.Name":     {"required", "max:50"},
		"Items.*.Quantity": {"required", "gte:1"},
	}
}

func TestNestedValid(t *testing.T) {
	r := &orderReq{Items: []Item{
		{Name: "widget", Quantity: 2},
		{Name: "gadget", Quantity: 1},
	}}
	if err := ValidateCtx(context.Background(), r); err != nil {
		t.Fatalf("want valid, got %v", err)
	}
}

func TestNestedOneFails(t *testing.T) {
	r := &orderReq{Items: []Item{
		{Name: "widget", Quantity: 2},
		{Name: "", Quantity: 1},
	}}
	err := ValidateCtx(context.Background(), r)
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("want validation error, got %v", err)
	}
	if _, has := ve.Fields["Items.1.Name"]; !has {
		t.Fatalf("want Items.1.Name flagged, got %v", ve.Fields)
	}
}

func TestNestedMultipleErrors(t *testing.T) {
	r := &orderReq{Items: []Item{
		{Name: "", Quantity: 0},
		{Name: "widget", Quantity: -1},
	}}
	err := ValidateCtx(context.Background(), r)
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("want validation error, got %v", err)
	}
	if _, has := ve.Fields["Items.0.Name"]; !has {
		t.Fatalf("want Items.0.Name flagged, got %v", ve.Fields)
	}
	if _, has := ve.Fields["Items.0.Quantity"]; !has {
		t.Fatalf("want Items.0.Quantity flagged, got %v", ve.Fields)
	}
	if _, has := ve.Fields["Items.1.Quantity"]; !has {
		t.Fatalf("want Items.1.Quantity flagged, got %v", ve.Fields)
	}
}

// --- form binding ---

type loginReq struct {
	Base
	Email    string   `form:"email"`
	Password string   `form:"password"`
	Roles    []string `form:"roles"`
}

func (r *loginReq) Rules() map[string][]string {
	return map[string][]string{
		"Email":    {"required", "email"},
		"Password": {"required", "min:8"},
		"Roles":    {"required", "min:1"},
	}
}

func TestFormValid(t *testing.T) {
	form := url.Values{
		"email":    {"a@b.com"},
		"password": {"hunter2hunter"},
		"roles":    {"admin", "editor"},
	}
	body := strings.NewReader(form.Encode())
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var dst loginReq
	if err := BindFormCtx(context.Background(), req, &dst); err != nil {
		t.Fatalf("want valid form, got %v", err)
	}
	if dst.Email != "a@b.com" {
		t.Fatalf("want email decoded, got %q", dst.Email)
	}
	if len(dst.Roles) != 2 || dst.Roles[0] != "admin" {
		t.Fatalf("want roles decoded, got %v", dst.Roles)
	}
}

func TestFormInvalid(t *testing.T) {
	form := url.Values{
		"email":    {"nope"},
		"password": {"x"},
	}
	body := strings.NewReader(form.Encode())
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var dst loginReq
	err := BindFormCtx(context.Background(), req, &dst)
	ve, ok := AsValidationError(err)
	if !ok {
		t.Fatalf("want validation error, got %v", err)
	}
	for _, f := range []string{"Email", "Password", "Roles"} {
		if _, has := ve.Fields[f]; !has {
			t.Fatalf("want %s flagged, got %v", f, ve.Fields)
		}
	}
}

func TestFormWrongContentType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	var dst loginReq
	if err := BindFormCtx(context.Background(), req, &dst); err == nil {
		t.Fatal("want error for wrong content type, got nil")
	}
}
