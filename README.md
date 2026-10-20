# reqval

Declarative form validation for [Go](https://go.dev), inspired by Laravel's Form Requests.

Instead of scattering validation rules, error formatting, and authorization checks across handlers, you define one struct per form, one tag per rule, and ask a single question:

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
- [Writing request structs](#writing-request-structs)
- [Using the helpers](#using-the-helpers)
  - [Bind and BindCtx](#bind-and-bindctx)
  - [BindGin](#bindgin)
  - [AbortWithError](#abortwitherror)
- [Tag reference](#tag-reference)
- [Custom rules](#custom-rules)
- [Database rules](#database-rules)
- [Authorization](#authorization)
- [Error handling](#error-handling)
- [Messages and translation](#messages-and-translation)
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

`reqval` centralizes those rules in one struct:

```go
type StorePostRequest struct {
    reqval.Base
    Title string `json:"title" validate:"required,max=255"`
    Slug  string `json:"slug"  validate:"required,unique=posts:slug"`
}
```

Now "what makes a valid post?" has one answer, in one file, testable in isolation.

**What this library gives you:**

- One struct per form, one tag per rule
- A `Bind` family of helpers that decode, authorize, and validate in one call
- Database-aware `unique` and `exists` rules with SQL injection protection
- Pre-validation authorization via an `Authorize(ctx) error` method
- Consistent 403 / 422 / 400 responses
- Human-readable messages, overridable per tag or per field
- A Gin adapter, plus a framework-agnostic core

**What it does not do:**

- It does not authenticate users. You provide the actor via context.
- It does not include roles or permissions. That is your domain logic.
- It does not render templates. The Gin adapter writes JSON; the core returns errors.

## Installation

```bash
go get github.com/thomas666-beast/reqval
```

Requires Go 1.22 or newer.

If you use the Gin adapter, also ensure Gin is present:

```bash
go get github.com/gin-gonic/gin
```

If you only use the framework-agnostic core (`Bind`, `BindCtx`, `Validate`, `ValidateCtx`), Gin is not compiled into your binary.

## Quick start

### Gin

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/thomas666-beast/reqval"
)

type StorePostRequest struct {
    reqval.Base
    Title string `json:"title" validate:"required,max=255"`
    Body  string `json:"body"  validate:"required,min=10"`
}

func main() {
    r := gin.Default()
    r.POST("/posts", func(c *gin.Context) {
        var req StorePostRequest
        if err := reqval.BindGin(c, &req); err != nil {
            reqval.AbortWithError(c, err)
            return
        }
        c.JSON(201, gin.H{"title": req.Title})
    })
    r.Run()
}
```

### net/http

```go
func CreatePost(w http.ResponseWriter, r *http.Request) {
    var req StorePostRequest
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
    json.NewEncoder(w).Encode(map[string]string{"title": req.Title})
}
```

### Database rules

For `unique` and `exists`, register a database once at startup:

```go
reqval.RegisterDB(db)
```

`db` can be `*sql.DB`, `*sql.Tx`, `sqlx.DB`, or anything with a matching `QueryRowContext` method.

## Writing request structs

Every request struct embeds `reqval.Base` and carries two kinds of tags: `json` for decoding, `validate` for rules.

```go
type StorePostRequest struct {
    reqval.Base

    Title      string `json:"title"       validate:"required,max=255"`
    Slug       string `json:"slug"        validate:"required,unique=posts:slug"`
    CategoryID int64  `json:"category_id" validate:"required,exists=categories:id"`
}
```

`reqval.Base` is a marker. It has no fields and no methods you need to call. Its only job is to satisfy the library's internal `Request` interface so the binding functions accept the struct.

Rules are comma-separated. `omitempty` applies to everything after it, so `omitempty,email` means "empty is fine, otherwise must be a valid email."

## Using the helpers

Three functions, each a thin layer around the previous one.

### Bind and BindCtx

```go
func Bind(r io.Reader, dst any) error
func BindCtx(ctx context.Context, r io.Reader, dst any) error
```

Framework-agnostic. `Bind` calls `BindCtx` with a background context, so use `BindCtx` when you need the request's context to reach the authorization hook or the `unique` rule's dynamic ignore ID.

```go
var req StorePostRequest
if err := reqval.BindCtx(r.Context(), r.Body, &req); err != nil {
    // handle
}
```

Both functions do three things in order:

1. Call `Authorize(ctx)` if the struct implements `reqval.Authorizer`. Denial → `*AuthorizationError`.
2. Decode JSON from the reader. Failure → the raw `json.Unmarshal` error.
3. Validate. Failure → `*ValidationError`.

### BindGin

```go
func BindGin(c *gin.Context, dst any) error
```

Same semantics as `BindCtx`, with two Gin-specific conveniences: it reads from `c.Request.Body` and `c.Request.Context()`, so you don't have to thread them in manually.

```go
func CreatePost(c *gin.Context) {
    var req StorePostRequest
    if err := reqval.BindGin(c, &req); err != nil {
        reqval.AbortWithError(c, err)
        return
    }
    c.JSON(201, gin.H{"title": req.Title})
}
```

### AbortWithError

```go
func AbortWithError(c *gin.Context, err error)
```

Inspects the error type and writes the appropriate status and body, then aborts the Gin context:

| Failure | Status | Body |
| :--- | :--- | :--- |
| `*AuthorizationError` | `403` | `{"error": "..."}` |
| `*ValidationError` | `422` | `{"errors": {"Field": "message"}}` |
| anything else | `400` | `{"error": "..."}` |

Calling it is optional. If you want a different response shape (e.g., a problem-details format), inspect the error yourself with `AsValidationError` / `AsAuthorizationError` and write whatever you like.

## Tag reference

### Built-in rules

These come from `go-playground/validator` unchanged.

| Tag | Example | Meaning |
| :--- | :--- | :--- |
| `required` | `validate:"required"` | Non-zero value |
| `min` | `validate:"min=3"` | Length or numeric value ≥ 3 |
| `max` | `validate:"max=255"` | Length or numeric value ≤ 255 |
| `email` | `validate:"email"` | Valid email address |
| `url` | `validate:"url"` | Valid URL |
| `uuid` | `validate:"uuid"` | Valid UUID |
| `numeric` | `validate:"numeric"` | String contains only digits |
| `alpha` | `validate:"alpha"` | Only letters |
| `alphanum` | `validate:"alphanum"` | Only letters and digits |
| `gte` | `validate:"gte=18"` | Value ≥ 18 |
| `lte` | `validate:"lte=120"` | Value ≤ 120 |
| `oneof` | `validate:"oneof=admin editor"` | One of the space-separated values |
| `omitempty` | `validate:"omitempty,email"` | Skip following rules if the field is empty |

### Rules added by reqval

These do not exist in `go-playground/validator` by default. `reqval` registers them.

| Tag | Example | Meaning |
| :--- | :--- | :--- |
| `in` | `validate:"in=admin editor"` | One of space- or colon-separated values |
| `between` | `validate:"between=1 100"` | Numeric range or string length range |
| `regexp` | `validate:"regexp=^[A-Z]{3}$"` | Matches a Go regular expression |
| `confirmed` | `validate:"confirmed"` | Equals `<Field>Confirmation` or `<field>_confirmation` |
| `unique` | `validate:"unique=users:username"` | Value not already in a database table |
| `exists` | `validate:"exists=categories:id"` | Value exists in a database table |

## Custom rules

### in

Accepts space- or colon-separated values.

```go
Role string `validate:"required,in=admin editor viewer"`
Role string `validate:"required,in=admin:editor:viewer"`
```

### between

Numeric range for numbers, length range for strings.

```go
Age   int    `validate:"between=18 120"`
Score int    `validate:"between=1 100"`
Name  string `validate:"between=3 10"` // length
```

### regexp

Matches against a Go regular expression. Non-string fields fail closed.

```go
Code string `validate:"regexp=^[A-Z]{3}-[0-9]{3}$"`
```

The name is `regexp`, not `regex`. An unregistered rule name makes `go-playground/validator` panic at first call, so if you see `Undefined validation function`, check the tag name against the table above.

### confirmed

Compares the field with a confirmation field. It tries two naming conventions:

1. `<FieldName>Confirmation` — e.g. `PasswordConfirmation`
2. `<fieldname>_confirmation` — e.g. `password_confirmation`

```go
Password             string `validate:"required,min=8,confirmed"`
PasswordConfirmation string
```

The confirmation field needs no `validate` tag; it only needs to be exported and named one of the forms above.

## Database rules

### Setup

```go
reqval.RegisterDB(db)
```

Registration is safe to call more than once. The most recent database wins. That is deliberate: it lets each test create its own in-memory database without tearing down the library.

### unique

```
unique=<table>:<column>
unique=<table>:<column>:<idColumn>
unique=<table>:<column>:<idColumn>:<ignoreID>
```

- **2 params** — plain uniqueness: `SELECT COUNT(*) FROM <table> WHERE <column> = ?`
- **3 params** — dynamic exclusion from context, via `reqval.WithIgnoreID(ctx, id)`. Falls back to plain uniqueness if no ID is in context. Recommended for create + update sharing one struct.
- **4 params** — hardcoded exclusion: `... WHERE <column> = ? AND <idColumn> <> <ignoreID>`

Update flow using the dynamic form:

```go
userID := userIDFromSession(c)
ctx := reqval.WithIgnoreID(c.Request.Context(), userID)

var req UpdateUserRequest
if err := reqval.BindCtx(ctx, c.Request.Body, &req); err != nil {
    reqval.AbortWithError(c, err)
    return
}
```

```go
Username string `validate:"required,unique=users:username:id"`
```

### exists

```
exists=<table>:<column>
```

```go
CategoryID int64 `validate:"required,exists=categories:id"`
```

Zero values skip the check; `required` catches them first. That means a zero-value foreign key produces "CategoryID is required", not "CategoryID is invalid", which is the message you want.

### SQL injection safety

Table and column names are validated against `^[a-zA-Z_][a-zA-Z0-9_]*$` before splicing. Anything containing `;`, quotes, or spaces fails closed. Example: `unique=users;DROP TABLE users:username` is rejected and the table survives.

### Tag parameter rules — read this first

**Comma separates rules in a `validate` tag.** The tag parser splits on comma before dispatching to a rule, so no custom validator ever sees a comma in its parameter. Multi-value rules use space or colon:

```go
Role string `validate:"in=admin editor"`   // correct
Role string `validate:"in=admin,editor"`   // wrong — "editor" becomes a rule name
Code string `validate:"between=1 100"`     // correct
Code string `validate:"between=1,100"`     // wrong
```

This applies to `in`, `between`, `unique`, `exists`, and every future custom rule.

## Authorization

Implement `Authorize(ctx) error` on the request struct. Return `nil` to allow, anything else to deny.

```go
func (r *StorePostRequest) Authorize(ctx context.Context) error {
    actor, ok := ActorFrom(ctx)
    if !ok {
        return fmt.Errorf("unauthenticated")
    }
    if actor.Role != "admin" {
        return fmt.Errorf("admin only")
    }
    return nil
}
```

Denial maps to `403`. Runs before decode, so unauthorized callers with malformed bodies get `403`, not `400`. That is deliberate: don't leak information about body parseability to callers who were never allowed to submit.

The actor comes from the request context. `reqval` does not invent an actor model; put your actor into the context before calling `BindCtx` / `BindGin`:

```go
type ctxKey int
const actorKey ctxKey = 1

ctx := context.WithValue(r.Context(), actorKey, Actor{ID: 1, Role: "admin"})
r = r.WithContext(ctx)
```

Then in your request struct:

```go
func ActorFrom(ctx context.Context) (Actor, bool) {
    a, ok := ctx.Value(actorKey).(Actor)
    return a, ok
}
```

## Error handling

Three error types, all from the `reqval` package.

| Type | Status | Unwrap with |
| :--- | :--- | :--- |
| `*AuthorizationError` | `403` | `reqval.AsAuthorizationError(err)` |
| `*ValidationError` | `422` | `reqval.AsValidationError(err)` |
| any other `error` | `400` | plain `err` |

### `*ValidationError`

```go
type ValidationError struct {
    Fields map[string]string // field name -> human-readable message
}

func (e *ValidationError) Error() string       // for logging
func (e *ValidationError) HTTPStatus() int     // always 422
```

### `*AuthorizationError`

```go
type AuthorizationError struct {
    Message string
}

func (e *AuthorizationError) Error() string    // the Message, or "forbidden"
func (e *AuthorizationError) HTTPStatus() int  // always 403
```

Discriminating in a handler:

```go
if err := reqval.BindGin(c, &req); err != nil {
    if ae, ok := reqval.AsAuthorizationError(err); ok {
        c.JSON(403, gin.H{"error": ae.Message})
        return
    }
    if ve, ok := reqval.AsValidationError(err); ok {
        c.JSON(422, gin.H{"errors": ve.Fields})
        return
    }
    c.JSON(400, gin.H{"error": err.Error()})
    return
}
```

That is exactly what `reqval.AbortWithError(c, err)` does.

### Panics vs. errors

The library does not panic on user input. The only panic you can trigger is a programming error: an unregistered rule name in a `validate` tag. `go-playground/validator` panics on that because a rule name is code, not data. If you see `Undefined validation function`, the tag name is wrong. The table above is authoritative.

## Messages and translation

Built-in rules have default English messages. Custom rules have their own defaults. You can override at three levels, resolved in this priority order:

1. **Field-specific override** — highest priority
2. **Tag-wide template** — middle
3. **Built-in default** — lowest

### Tag-wide override

```go
reqval.RegisterMessage("required", "Please fill in {{field}}.")
```

Placeholders: `{{field}}` (the Go field name) and `{{param}}` (the rule's parameter, e.g. the number in `max=255`).

### Field-specific override

```go
reqval.RegisterFieldMessage("StorePostRequest", "Title", "We really need a title.")
```

Field name is the Go field name (`Title`), not the JSON tag name (`title`).

### Translation bundle

```go
reqval.LoadZH() // Chinese defaults for the built-in rules
```

After `LoadZH()`, all built-in tags render Chinese messages. Field-specific overrides still win, as the priority order says.

Writing your own bundle is just a series of `RegisterMessage` calls:

```go
func LoadFR() {
    reqval.RegisterMessage("required", "Le champ {{field}} est obligatoire.")
    reqval.RegisterMessage("max", "Le champ {{field}} ne peut pas dépasser {{param}} caractères.")
}
```

## Examples

Runnable programs in `examples/`:

| Directory | Shows |
| :--- | :--- |
| `basic/` | Minimal validate call |
| `errors/` | 422 error shape |
| `http/` | net/http binding |
| `gin/` | Gin binding |
| `messages/` | Message overrides |
| `unique/` | Create + update uniqueness |
| `exists/` | Foreign-key check |
| `authz/` | 403 before decode |
| `full/` | All features together |
| `dynamic/` | Dynamic ignore ID |
| `rules/` | Full rule set |

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

## FAQ

### Why `reqval` and not just `go-playground/validator`?

`go-playground/validator` is the rule engine. `reqval` is the glue around it: decode, authorize, validate, format errors, map to status codes, and thread a database in for `unique` / `exists`. You can absolutely use validator directly and wire all that up yourself. `reqval` is the wiring, packaged.

### Why do I need to embed `reqval.Base`?

It is a marker interface. The binding functions need a common type to accept, and embedding a zero-sized struct is the least intrusive way to provide one. You never call anything on it, and it adds nothing to your struct's size.

### Why is authorization before decode?

Order matters for security. A non-admin sending malformed JSON should get `403`, not `400`, because they were never allowed to submit. Decoding first would leak whether their JSON was well-formed, which is information they shouldn't have. Laravel does the same thing with `FormRequest::authorize()`.

### Can a policy method call another policy method?

There are no policy classes in `reqval` — authorization lives on the request struct itself. If you want shared logic, extract a helper function and call it from multiple `Authorize` methods. That is simpler than a policy layer and produces the same result.

### How do I check permissions without an object?

`reqval`'s authorization is about *requests*, not *objects*. If you need a check that has no request body — "is this user an admin?" — do it in middleware, before the request reaches the handler. `reqval` is the right tool when the answer depends on what the request body says.

### How do I handle unauthenticated users?

Return an error from `Authorize` when the actor is missing. That maps to `403`. If you'd prefer unauthenticated users to be redirected to a login page, check the actor in your handler before calling `BindCtx`:

```go
if ActorFrom(r.Context()) == nil {
    http.Redirect(w, r, "/login", http.StatusSeeOther)
    return
}
```

`reqval` does not impose a policy on this; you decide how unauthenticated access is handled.

### Can I use this with sqlx, GORM, pgx?

Yes for `sqlx` — `*sqlx.DB` satisfies `DBQuerier` directly. For GORM and pgx, wrap the underlying connection in a small adapter that exposes `QueryRowContext(ctx, query, args ...any) *sql.Row`. The library only ever calls that one method.

### What about `FormRequest`-style per-form classes?

In Go, a "per-form class" is a struct. That struct *is* your FormRequest. There is no second layer to learn.

### Why does `regexp` panic if I write `regex`?

Because `regex` is not a registered rule name. `go-playground/validator` panics on unknown rule names because a rule name is code, not data — it can't fall back to a runtime error without hiding typos. The fix is to write `regexp`.

## License

This library is free software; you can redistribute it and/or modify it under the same terms as Go itself.
