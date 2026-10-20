# reqval

Declarative form validation for [Go](https://go.dev), inspired by Laravel's Form Requests.

Instead of scattering validation rules, error formatting, and authorization checks across handlers, you define one request type per form. That file contains the rules, the messages, and the authorization. The controller just calls it.

```go
if err := reqval.BindGin(c, &req); err != nil {
    reqval.AbortWithError(c, err)
    return
}
```

If the input is valid, the handler proceeds. If not, the client gets a 422 with field-level messages. If authorization fails, a 403. If the JSON is malformed, a 400. One call, three consistent outcomes.

## Table of contents

- [Why this library](#why-this-library)
- [Installation](#installation)
- [Quick start](#quick-start)
- [Writing a request](#writing-a-request)
- [Rules](#rules)
- [Messages](#messages)
- [Authorization](#authorization)
- [Binding](#binding)
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

If you use the Gin adapter:

```bash
go get github.com/gin-gonic/gin
```

If you only use the framework-agnostic core (`Bind`, `BindCtx`, `Validate`, `ValidateCtx`), Gin is not compiled into your binary.

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
    return nil // no auth check for this form
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

Every request type implements the `reqval.Request` interface:

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
    reqval.Base // Rules/Messages/Authorize all return nil
    Name        string `json:"name"`
}

func (r *SimpleRequest) Rules() map[string][]string {
    return map[string][]string{"Name": {"required"}}
}
```

That is the whole contract. No tags, no config, no magic. The request file is self-contained.

## Rules

`Rules()` returns a map from field name to a slice of rule strings. Each rule is one of:

- A **name** — `"required"`
- A **name and parameter** — `"max:255"`, `"unique:users,email"`
- A **name and comma-separated parameters** — `"between:1,100"`, `"unique:users,email,id,42"`

### Built-in rules

| Rule | Example | Meaning |
| :--- | :--- | :--- |
| `required` | `"required"` | Non-zero value |
| `min` | `"min:3"` | Length or numeric value ≥ 3 |
| `max` | `"max:255"` | Length or numeric value ≤ 255 |
| `email` | `"email"` | Valid email address |
| `url` | `"url"` | Valid URL |
| `uuid` | `"uuid"` | Valid UUID |
| `numeric` | `"numeric"` | String contains only digits |
| `alpha` | `"alpha"` | Only letters |
| `alphanum` | `"alphanum"` | Only letters and digits |
| `gte` | `"gte:18"` | Value ≥ 18 |
| `lte` | `"lte:120"` | Value ≤ 120 |
| `in` | `"in:admin,editor"` | One of comma-separated values |
| `between` | `"between:1,100"` | Numeric range or string length range |
| `regexp` | `"regexp:^[A-Z]{3}$"` | Matches a Go regular expression |
| `confirmed` | `"confirmed"` | Equals `<Field>Confirmation` |

### Database rules

| Rule | Example | Meaning |
| :--- | :--- | :--- |
| `unique` | `"unique:users,email"` | Not already in table |
| `exists` | `"exists:categories,id"` | Exists in table |

See [Database rules](#database-rules) for details.

### Multiple rules

Fields run rules in order and stop at the first failure:

```go
func (r *StorePostRequest) Rules() map[string][]string {
    return map[string][]string{
        "Email": {"required", "email"},
        "Age":   {"required", "gte:18", "lte:120"},
    }
}
```

## Messages

`Messages()` returns a map from `<field>.<rule>` to a message template. Three priority levels:

1. **Request's `Messages()`** — per form
2. **Application default bundle** — set with `reqval.SetDefaultMessages(...)`
3. **Library fallback** — `"The <field> field is invalid."` (used only when 1 and 2 both miss)

### Per-request messages

```go
func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "Title.required": "We really need a title.",
        "Body.min":       "The body must be at least {{param}} characters.",
    }
}
```

Placeholders: `{{field}}` (the field name) and `{{param}}` (the rule's parameter).

### Application default bundle

Set once at startup, usually with a translation:

```go
reqval.SetDefaultMessages(reqval.ZH()) // Chinese defaults for all requests
```

The `*.rule` form applies a template to every field:

```go
func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "*.required": "Please fill in {{field}}.",
    }
}
```

### The library contains no rule-specific English

This is deliberate. Translating means calling `reqval.SetDefaultMessages(zhBundle)` or returning a different bundle from `Messages()` — not editing the library. The only string the library itself emits is `"The <field> field is invalid."`, used as a last resort when the user provided nothing.

## Authorization

`Authorize(ctx)` runs **before** decoding, so a non-admin with malformed JSON gets 403, not 400. Implement it on the request type:

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

Return `nil` to allow, any error to deny. The library wraps the error in `*AuthorizationError` with HTTP status 403.

The actor comes from the request context. `reqval` does not invent an actor model; put your actor in the context before calling `BindCtx` / `BindGin`:

```go
type ctxKey int
const actorKey ctxKey = 1

ctx := context.WithValue(r.Context(), actorKey, Actor{ID: 1, Role: "admin"})
r = r.WithContext(ctx)
```

For requests with no auth check, omit the method (inherited from `reqval.Base`) or return `nil`.

## Binding

Three functions, from most generic to most convenient:

| Function | Signature | Use |
| :--- | :--- | :--- |
| `Bind` | `Bind(r io.Reader, dst any) error` | Background context |
| `BindCtx` | `BindCtx(ctx context.Context, r io.Reader, dst any) error` | With context |
| `BindGin` | `BindGin(c *gin.Context, dst any) error` | Gin adapter |

All three do the same three steps in order:

1. Call `Authorize(ctx)`. Denial → `*AuthorizationError` (403).
2. Decode JSON. Failure → the raw `json.Unmarshal` error (400).
3. Validate. Failure → `*ValidationError` (422).

### Choosing your response shape

The controller decides how to render the error. Two options:

**Option A — the built-in helper (convenient):**

```go
if err := reqval.BindGin(c, &req); err != nil {
    reqval.AbortWithError(c, err)
    return
}
```

`AbortWithError` writes 403 / 422 / 400 with a JSON body and aborts the Gin context.

**Option B — inspect the error yourself (flexible):**

```go
if err := reqval.BindGin(c, &req); err != nil {
    if ve, ok := reqval.AsValidationError(err); ok {
        // custom rendering: flash, redirect, HTML, whatever you want
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

## Error handling

Three error types:

| Type | Status | When | Unwrap with |
| :--- | :--- | :--- | :--- |
| `*AuthorizationError` | `403` | `Authorize` returned non-nil | `reqval.AsAuthorizationError(err)` |
| `*ValidationError` | `422` | A rule failed | `reqval.AsValidationError(err)` |
| other | `400` | JSON decode failure, empty body, I/O error | plain `err` |

`ValidationError.Fields` is `map[string]string` — field name to message.

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

Zero values skip the check; `required` catches them first.

### SQL injection safety

Table and column names are validated against `^[a-zA-Z_][a-zA-Z0-9_]*$` before splicing. Anything containing `;`, quotes, or spaces fails closed.

## Examples

Runnable programs in `examples/`:

| Directory | Shows |
| :--- | :--- |
| `basic/` | Minimal validate call |
| `errors/` | 422 error shape |
| `http/` | net/http binding |
| `gin/` | Gin binding |
| `messages/` | Fallback, bundle, and override in one file |
| `rules/` | The full rule set |

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
| `php artisan make:request X` | Not implemented yet | See [Roadmap](#roadmap) |
| Rules in `rules()` method | Rules in `Rules()` method | Same idea, Go syntax |
| Messages in `messages()` method | Messages in `Messages()` method | Same idea, Go syntax |
| `authorize()` returns bool | `Authorize()` returns `error` | Errors carry messages for the 403 body |
| Global `lang/` files | `SetDefaultMessages(bundle)` | Explicit, testable, no filesystem coupling |
| Rules computed at runtime (`Rule::unique()->ignore($id)`) | Runtime via `strconv.FormatInt(r.ID, 10)` in `Rules()` | Idiomatic Go; no magic injection |
| Response via `ValidationException` handler | Error returned; controller decides | Go has no global handler; explicit is better |

The one significant missing feature is the `make:request` generator. The rest is either matched or intentionally adapted to Go.

## Roadmap

Not implemented yet:

- `reqval new request <Name>` scaffold command (`make:request` equivalent)
- `BindForm` for `application/x-www-form-urlencoded`
- Fiber adapter
- `nullable`, `sometimes`, `required_if`
- `unique` with additional `where` conditions
- Richer `exists` variants (`exists:table,column,where:active,1`)

## FAQ

### Why not just use `go-playground/validator`?

Because that gives you the rule engine, not the glue. `reqval` wraps it with the Form Request pattern: one request file per form, messages colocated, authorization before decode, consistent error types, and a controller that doesn't mention validation at all.

### Why does the request embed `reqval.Base`?

To satisfy the `Request` interface with empty defaults so you only write the methods you need. It is a zero-sized struct — embedding it costs nothing.

### Why do rules live in a method, not struct tags?

Tags are static: computed at compile time, cannot depend on runtime values. Rules in a method can — including the current row ID for update uniqueness checks. Laravel made the same choice with `rules()`.

### Why does authorization run before decode?

A non-admin sending malformed JSON should get 403, not 400. Decoding first would leak information about body parseability to callers who were never allowed to submit.

### Why is there no `can()` helper like Laravel's?

Because Go's method dispatch and Gin's context don't have a slot for one. In Laravel, `$this->can(...)` works because the base class knows the user. In `reqval`, the actor is in the context, and policies are not a concept the library models. If you need object-level authorization (not request-level), that's a separate library — `reqval` is about requests.

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

Yes. `BindCtx(r.Context(), r.Body, &req)` returns the same error types. Render them however you like:

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

### Why does the `gin` example show fallback messages?

If you run `go run ./examples/gin` and see `"The Title field is invalid."`, that example is missing a `Messages()` method on its request type and no default bundle was installed. Add `Messages()` or call `SetDefaultMessages` at startup. The `messages` example shows all three resolution levels side by side.

## License

MIT.
