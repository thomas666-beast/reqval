# reqval

Declarative form validation for [Go](https://go.dev), inspired by Laravel's Form Requests.

Instead of scattering validation rules, error formatting, and authorization checks across handlers, you define one request type per form. That file contains the rules, the messages, the authorization, and (optionally) file rules. The controller just calls it.

```go
if err := reqval.BindGin(c, &req); err != nil {
    reqval.AbortWithError(c, err)
    return
}
```

Valid input: the handler proceeds. Invalid: 422 with field-level messages. Unauthorized: 403. Malformed body: 400. One call, one consistent shape.

## Table of contents

- [Why this library](#why-this-library)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Writing a request](#writing-a-request)
- [Rules](#rules)
- [Nested paths](#nested-paths)
- [File uploads](#file-uploads)
- [Messages](#messages)
- [Authorization](#authorization)
- [Binding](#binding)
- [Scaffolding](#scaffolding)
- [Error handling](#error-handling)
- [Database rules](#database-rules)
- [Examples](#examples)
- [Testing](#testing)
- [Differences from Laravel](#differences-from-laravel)
- [FAQ](#faq)
- [License](#license)

## Why this library

Validation is easy to get wrong. A typical Go handler ends up with code like this:

```go
var req StorePostRequest
if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(400, gin.H{"error": err.Error()})
    return
}
if req.Title == "" {
    c.JSON(422, gin.H{"error": "title is required"})
    return
}
if len(req.Title) > 255 {
    c.JSON(422, gin.H{"error": "title too long"})
    return
}
// ... twenty more lines
```

This works until you have five handlers, three resources, and a database uniqueness check. Then the rules drift, some handlers forget a check, and every endpoint returns errors in a slightly different shape.

`reqval` centralizes those rules in one request file:

```go
type StorePostRequest struct {
    reqval.Base
    Title string `json:"title"`
    Body  string `json:"body"`
}

func (r *StorePostRequest) Rules() map[string][]string {
    return map[string][]string{
        "Title": {"required", "max:255"},
        "Body":  {"required", "min:10"},
    }
}

func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "Title.required": "We really need a title.",
    }
}

func (r *StorePostRequest) Authorize(ctx context.Context) error {
    actor, ok := ActorFrom(ctx)
    if !ok || actor.Role != "admin" {
        return errors.New("admin only")
    }
    return nil
}
```

Now "what makes a valid post?" has one answer, in one file, testable in isolation.

## Installation

```bash
go get github.com/thomas666-beast/reqval
```

Requires Go 1.22 or newer.

For the Gin adapter:

```bash
go get github.com/gin-gonic/gin
```

If you only use the framework-agnostic core (`Bind`, `BindCtx`, `BindForm`, `BindFormCtx`, `BindMultipartCtx`, `Validate`, `ValidateCtx`), Gin is not compiled into your binary.

## Quick start

### The request file

```go
package requests

import (
    "context"
    "github.com/thomas666-beast/reqval"
)

type StorePostRequest struct {
    reqval.Base
    Title string `json:"title"`
    Body  string `json:"body"`
}

func (r *StorePostRequest) Rules() map[string][]string {
    return map[string][]string{
        "Title": {"required", "max:255"},
        "Body":  {"required", "min:10"},
    }
}

func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "Title.required": "We really need a title.",
    }
}

func (r *StorePostRequest) Authorize(ctx context.Context) error {
    return nil
}
```

### The controller

```go
func CreatePost(c *gin.Context) {
    var req requests.StorePostRequest
    if err := reqval.BindGin(c, &req); err != nil {
        reqval.AbortWithError(c, err)
        return
    }
    c.JSON(201, gin.H{"title": req.Title})
}
```

The controller never mentions `req.Title == ""` again. That's the point.

## Writing a request

Every request type implements `reqval.Request`:

```go
type Request interface {
    Rules() map[string][]string
    Messages() map[string]string
    Authorize(ctx context.Context) error
}
```

Embed `reqval.Base` to satisfy all three with empty defaults, then override what you need:

```go
type SimpleRequest struct {
    reqval.Base
    Name        string `json:"name"`
}

func (r *SimpleRequest) Rules() map[string][]string {
    return map[string][]string{"Name": {"required"}}
}
```

That is the whole contract. No tags, no config, no magic. The request file is self-contained.

Requests that accept file uploads implement one more optional interface:

```go
type FileRequest interface {
    FileRules() []FileRule
}
```

## Rules

`Rules()` returns `map[string][]string`: field name (or path) to a slice of rule strings. Each rule is one of:

- A **name** — `"required"`
- A **name and parameter** — `"max:255"`
- A **name and comma-separated parameters** — `"between:1,100"`, `"unique:users,email,id,42"`

Fields run rules in order and stop at the first failure.

### Built-in rules

| Rule | Example | Meaning |
| :--- | :--- | :--- |
| `required` | `"required"` | Non-zero value; non-empty slice/map/array |
| `min` | `"min:3"` | Length, element count, or numeric value ≥ 3 |
| `max` | `"max:255"` | Length, element count, or numeric value ≤ 255 |
| `email` | `"email"` | Valid email address |
| `url` | `"url"` | Valid URL |
| `uuid` | `"uuid"` | Valid UUID |
| `numeric` | `"numeric"` | String contains only digits |
| `alpha` | `"alpha"` | Only letters |
| `alphanum` | `"alphanum"` | Only letters and digits |
| `gte` | `"gte:18"` | Value ≥ 18 |
| `lte` | `"lte:120"` | Value ≤ 120 |
| `in` | `"in:admin,editor"` | One of the given values |
| `oneof` | `"oneof:admin editor"` | One of the given values (space-separated) |
| `between` | `"between:1,100"` | Numeric range or string length range |
| `regexp` | `"regexp:^[A-Z]{3}$"` | Matches a Go regular expression |
| `date` | `"date:2006-01-02"` | Parses as a date in the given Go layout. Empty skips; omit layout → `2006-01-02` |
| `confirmed` | `"confirmed"` | Equals `<Field>Confirmation` or `<field>_confirmation` |

### Database rules

| Rule | Example | Meaning |
| :--- | :--- | :--- |
| `unique` | `"unique:users,email"` | Not already in table |
| `unique` (ignore) | `"unique:users,email,id,42"` | Same, excluding row where `id = 42` |
| `exists` | `"exists:categories,id"` | Exists in table |

See [Database rules](#database-rules) for details.

### Multiple rules

```go
func (r *StorePostRequest) Rules() map[string][]string {
    return map[string][]string{
        "Email": {"required", "email"},
        "Age":   {"required", "gte:18", "lte:120"},
    }
}
```

## Nested paths

Rule keys may be paths. Dot separates segments. `*` iterates a slice, array, or map.

```go
type Item struct {
    Name     string `json:"name"`
    Quantity int    `json:"quantity"`
}

type OrderRequest struct {
    reqval.Base
    Items []Item `json:"items"`
}

func (r *OrderRequest) Rules() map[string][]string {
    return map[string][]string{
        "Items.*.Name":     {"required", "max:50"},
        "Items.*.Quantity": {"required", "gte:1"},
    }
}
```

If the second item's `Name` is empty, the error key is `Items.1.Name`. If the third item's `Quantity` is `0`, the key is `Items.2.Quantity`. That is, errors point at the specific element, not the whole slice.

Path syntax:

- `Items.*.Name` — every element's `Name` field
- `Items.0.Name` — a specific element (0-indexed)
- `Tags.*.Value` — map entries (key is stringified into the path)

`confirmed` works inside a path: `Password` on each element compares against `PasswordConfirmation` on the same element.

## File uploads

Implement `FileRules()` to declare constraints on uploaded files. Use `BindMultipartCtx` (or `BindMultipartGin`) for `multipart/form-data`.

```go
type UploadRequest struct {
    reqval.Base
    Description string `form:"description"`
}

func (r *UploadRequest) Rules() map[string][]string {
    return map[string][]string{
        "Description": {"required", "max:200"},
    }
}

func (r *UploadRequest) FileRules() []reqval.FileRule {
    return []reqval.FileRule{
        {Field: "avatar", MaxBytes: 2 << 20, MIMEs: []string{"image/png", "image/jpeg"}},
    }
}
```

```go
func Upload(c *gin.Context) {
    var req UploadRequest
    if err := reqval.BindMultipartGin(c, &req); err != nil {
        reqval.AbortWithError(c, err)
        return
    }
    // req.Description is validated; the file passed MIME and size checks.
}
```

`FileRule` fields:

| Field | Meaning |
| :--- | :--- |
| `Field` | Form field name (`avatar`) |
| `MaxBytes` | Upper size limit (0 = no limit) |
| `MinBytes` | Lower size limit (0 = no minimum) |
| `MIMEs` | Allowed MIME types (empty = any) |

MIME detection reads the first 512 bytes with `http.DetectContentType`, not the client-supplied `Content-Type` header. A file with a `.png` extension but plain-text contents is rejected with the `mimes` message.

## Messages

`Messages()` returns `map[string]string` keyed by `<field>.<rule>` or `*.<rule>`. Three priority levels:

1. **Request's `Messages()`** — per form
2. **Application default bundle** — set with `reqval.SetDefaultMessages(...)`
3. **Library fallback** — `"The <field> field is invalid."`

### Per-request messages

```go
func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "Title.required": "We really need a title.",
        "Body.min":       "The body must be at least {{param}} characters.",
    }
}
```

Placeholders: `{{field}}` (the bare field name) and `{{param}}` (the rule's parameter).

### Application default bundle

Set once at startup, usually with a translation:

```go
reqval.SetDefaultMessages(reqval.ZH()) // Chinese defaults for all requests
```

The `*.<rule>` form applies a template to every field:

```go
func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "*.required": "Please fill in {{field}}.",
    }
}
```

### The library contains no rule-specific English

This is deliberate. Translating means calling `reqval.SetDefaultMessages(zhBundle)` or returning a different bundle from `Messages()`, not editing the library. The only string the library itself emits is `"The <field> field is invalid."`, used as a last resort when the user provided nothing.

## Authorization

`Authorize(ctx)` runs **before** decoding, so a non-admin with malformed JSON gets 403, not 400.

```go
func (r *StorePostRequest) Authorize(ctx context.Context) error {
    actor, ok := ActorFrom(ctx)
    if !ok {
        return errors.New("unauthenticated")
    }
    if actor.Role != "admin" {
        return errors.New("admin only")
    }
    return nil
}
```

Return `nil` to allow, any error to deny. The library wraps the error in `*AuthorizationError` (HTTP 403). The error's message becomes the 403 body's `error` field.

The actor comes from the request context. `reqval` does not invent an actor model; put your actor into the context before calling the bind functions:

```go
type ctxKey int
const actorKey ctxKey = 1

ctx := context.WithValue(r.Context(), actorKey, Actor{ID: 1, Role: "admin"})
r = r.WithContext(ctx)
```

Requests with no auth check can omit the method (inherited from `reqval.Base`) or return `nil`.

## Binding

Seven functions across three body formats:

### JSON

| Function | Signature | Use |
| :--- | :--- | :--- |
| `Bind` | `Bind(r io.Reader, dst any) error` | Background context |
| `BindCtx` | `BindCtx(ctx context.Context, r io.Reader, dst any) error` | With context |
| `BindGin` | `BindGin(c *gin.Context, dst any) error` | Gin adapter |

### Form (`application/x-www-form-urlencoded`)

| Function | Signature | Use |
| :--- | :--- | :--- |
| `BindForm` | `BindForm(r io.Reader, contentType string, dst any) error` | Body in hand |
| `BindFormCtx` | `BindFormCtx(ctx context.Context, r *http.Request, dst any) error` | `*http.Request` |
| `BindFormGin` | `BindFormGin(c *gin.Context, dst any) error` | Gin adapter |

### Multipart (`multipart/form-data`)

| Function | Signature | Use |
| :--- | :--- | :--- |
| `BindMultipartCtx` | `BindMultipartCtx(ctx context.Context, r *http.Request, dst any) error` | `*http.Request` |
| `BindMultipartGin` | `BindMultipartGin(c *gin.Context, dst any) error` | Gin adapter |

All three formats do the same three steps in order:

1. Call `Authorize(ctx)`. Denial → `*AuthorizationError` (403).
2. Decode the body. Failure → a decode error (400).
3. Validate. Failure → `*ValidationError` (422).

### Choosing your response shape

The controller decides how to render the error.

**Option A — the built-in helper:**

```go
if err := reqval.BindGin(c, &req); err != nil {
    reqval.AbortWithError(c, err)
    return
}
```

Writes 403 / 422 / 400 with a JSON body and aborts the Gin context.

**Option B — inspect the error yourself:**

```go
if err := reqval.BindGin(c, &req); err != nil {
    if ve, ok := reqval.AsValidationError(err); ok {
        // flash, redirect, HTML, whatever you want
        c.Set("errors", ve.Fields)
        c.Redirect(302, "/posts/create")
        return
    }
    if ae, ok := reqval.AsAuthorizationError(err); ok {
        c.JSON(403, gin.H{"error": ae.Message})
        return
    }
    c.JSON(400, gin.H{"error": err.Error()})
    return
}
```

This is the option that lets you flash errors on the session instead of returning JSON — the controller decides the medium.

## Scaffolding

A small CLI generates a request file with `Rules()`, `Messages()`, and `Authorize()` stubbed out — the `make:request` equivalent.

```bash
go install github.com/thomas666-beast/reqval/cmd/reqval@latest
reqval new request StorePostRequest
```

Creates `storepostrequest.go` in the current directory:

```go
package requests

import (
    "context"
    "github.com/thomas666-beast/reqval"
)

type StorePostRequest struct {
    reqval.Base
}

func (r *StorePostRequest) Rules() map[string][]string {
    return map[string][]string{}
}

func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{}
}

func (r *StorePostRequest) Authorize(ctx context.Context) error {
    return nil
}
```

Flags:

| Flag | Default | Meaning |
| :--- | :--- | :--- |
| `--package` | `requests` | Package name |
| `--dir` | `.` | Output directory |
| `--force` | `false` | Overwrite if file exists |

```bash
reqval new request UpdatePostRequest --package=posts --dir=internal/http/requests
```

## Error handling

Three error types:

| Type | Status | When | Unwrap with |
| :--- | :--- | :--- | :--- |
| `*AuthorizationError` | `403` | `Authorize` returned non-nil | `reqval.AsAuthorizationError(err)` |
| `*ValidationError` | `422` | A rule failed | `reqval.AsValidationError(err)` |
| other | `400` | JSON decode failure, empty body, I/O error, wrong content type | plain `err` |

`ValidationError.Fields` is `map[string]string` — field name (or path) to message.

## Database rules

### Setup

For `unique` and `exists`, register a database once at startup:

```go
reqval.RegisterDB(db)
```

`db` can be `*sql.DB`, `*sql.Tx`, `sqlx.DB`, or anything with a matching `QueryRowContext` method:

```go
type DBQuerier interface {
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
```

Registration is safe to call more than once. The most recent database wins.

### unique

```
"unique:<table>,<column>"
"unique:<table>,<column>,<idColumn>,<ignoreID>"
```

- **2 params** — plain uniqueness: `SELECT COUNT(*) FROM <table> WHERE <column> = ?`
- **4 params** — exclusion: `... WHERE <column> = ? AND <idColumn> <> <ignoreID>`. Use on update to exclude the row you're editing.

```go
func (r *StoreUserRequest) Rules() map[string][]string {
    return map[string][]string{
        "Username": {"required", "unique:users,username"},
    }
}

func (r *UpdateUserRequest) Rules() map[string][]string {
    return map[string][]string{
        "Username": {"required", "unique:users,username,id," + strconv.FormatInt(r.ID, 10)},
    }
}
```

Empty strings and zero values skip the check; `required` catches them first.

### exists

```
"exists:<table>,<column>"
```

```go
func (r *StorePostRequest) Rules() map[string][]string {
    return map[string][]string{
        "CategoryID": {"required", "exists:categories,id"},
    }
}
```

Zero values skip; `required` catches them first. That means a zero-value foreign key produces "CategoryID is required", not "CategoryID is invalid", which is the message you want.

### SQL injection safety

Table and column names are validated against `^[a-zA-Z_][a-zA-Z0-9_]*$` before splicing. Anything containing `;`, quotes, or spaces fails closed. Example: `"unique:users;DROP TABLE users,username"` is rejected and the table survives.

## Examples

Runnable programs in `examples/`:

| Directory | Shows |
| :--- | :--- |
| `basic/` | Minimal validate call |
| `errors/` | 422 error shape |
| `http/` | net/http binding |
| `gin/` | Gin binding |
| `form/` | Form-urlencoded binding |
| `upload/` | File upload MIME + size |
| `messages/` | Fallback, bundle, and override |
| `rules/` | The full rule set |
| `unique/` | Unique create + update |
| `exists/` | Foreign-key check |
| `authz/` | 403 before decode |
| `dynamic/` | Dynamic ignore ID |
| `full/` | All features together |

Run any of them:

```bash
go run ./examples/rules
```

## Testing

```bash
go vet ./...
go test -timeout 60s -v ./...
go test -timeout 60s -race ./...
```

All tests must print `PASS` and `ok github.com/thomas666-beast/reqval`.

## Differences from Laravel

`reqval` is inspired by Laravel's Form Requests, not a clone. The differences are deliberate.

| Laravel | reqval | Why |
| :--- | :--- | :--- |
| `php artisan make:request X` | `reqval new request X` | Same idea; a small CLI, not a full Artisan |
| Rules in `rules()` method | Rules in `Rules()` method | Same idea, Go syntax |
| Messages in `messages()` method | Messages in `Messages()` method | Same idea, Go syntax |
| `authorize()` returns bool | `Authorize()` returns `error` | Errors carry messages for the 403 body |
| Global `lang/` files | `SetDefaultMessages(bundle)` | Explicit, testable, no filesystem coupling |
| Rules computed at runtime (`Rule::unique()->ignore($id)`) | Runtime via string building in `Rules()` | Idiomatic Go; no magic injection |
| Response via `ValidationException` handler | Error returned; controller decides | Go has no global handler; explicit is better |
| `items.*.name` for arrays | `Items.*.Name` for slices | Same syntax; Go field names are exported |

## FAQ

### Why not just use `go-playground/validator`?

Because that gives you the rule engine, not the glue. `reqval` wraps it with the Form Request pattern: one request file per form, messages colocated, authorization before decode, consistent error types, and a controller that doesn't mention validation at all. `go-playground/validator` is still a transitive dependency — `reqval` uses it for the rule engine — but the API is different.

### Why does the request embed `reqval.Base`?

To satisfy the `Request` interface with empty defaults so you only write the methods you need. It is a zero-sized struct — embedding it costs nothing.

### Why do rules live in a method, not struct tags?

Tags are static: computed at compile time, cannot depend on runtime values. Rules in a method can — including the current row ID for update uniqueness checks. Laravel made the same choice with `rules()`.

### Why does authorization run before decode?

A non-admin sending malformed JSON should get 403, not 400. Decoding first would leak information about body parseability to callers who were never allowed to submit.

### How do I translate messages?

```go
reqval.SetDefaultMessages(reqval.ZH())
```

Or per-request:

```go
func (r *StorePostRequest) Messages() map[string]string {
    return reqval.ZH()
}
```

For a custom language, write a function that returns `map[string]string` and pass it to `SetDefaultMessages`. The library contains no rule-specific English, so there is nothing in the source to edit.

### Can I use `reqval` with `net/http` directly?

Yes. Use `BindCtx(r.Context(), r.Body, &req)` for JSON, `BindFormCtx(r.Context(), r, &req)` for form bodies, `BindMultipartCtx(r.Context(), r, &req)` for uploads. All return the same error types.

```go
if err := reqval.BindCtx(r.Context(), r.Body, &req); err != nil {
    if ve, ok := reqval.AsValidationError(err); ok {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(422)
        json.NewEncoder(w).Encode(map[string]any{"errors": ve.Fields})
        return
    }
    http.Error(w, err.Error(), 400)
    return
}
```

### How do I check permissions without an object?

`reqval` is about requests, not objects. If you need a check that has no request body — "is this user an admin?" — do it in middleware, before the request reaches the handler. `reqval` is the right tool when the answer depends on what the request body says.

### What about `min` on a slice?

`min` and `max` on a slice, array, or map mean the number of elements, not a numeric value. `"Items": {"min:1"}` means "at least one item". The same rules on a string mean length; on a number mean value.

## License

MIT.
