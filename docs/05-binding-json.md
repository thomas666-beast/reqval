# Binding JSON

Three functions bind a JSON body to a request struct: `Bind`, `BindCtx`, and `BindGin`. They differ only in how they receive the input and the context. Their semantics — authorize, decode, validate — are identical.

This file explains when to use each and how to render the result.

## The three functions

| Function | Signature | Input | Context |
| :--- | :--- | :--- | :--- |
| `Bind` | `Bind(r io.Reader, dst any) error` | An `io.Reader` | `context.Background()` |
| `BindCtx` | `BindCtx(ctx context.Context, r io.Reader, dst any) error` | An `io.Reader` | Your context |
| `BindGin` | `BindGin(c *gin.Context, dst any) error` | A Gin context | `c.Request.Context()` |

All three do the same thing:

1. If `dst` implements `reqval.Authorizer` (via `Authorize(ctx)`), call it. On error, return `*AuthorizationError`.
2. Read the body. If empty, return `ErrEmptyBody`.
3. Decode JSON. On error, return the raw `json.Unmarshal` error.
4. Call `Validate` / `ValidateCtx`. On failure, return `*ValidationError`.

## Which one to use

**`BindGin` in a Gin handler.** You are already inside a handler with a `*gin.Context`, and you want the request context (with your actor, request ID, etc.) to reach `Authorize`.

**`BindCtx` in `net/http` or any other framework.** You have the `*http.Request`; pass `r.Context()` and `r.Body`:

```go
if err := reqval.BindCtx(r.Context(), r.Body, &req); err != nil {
    // ...
}
```

**`Bind` when the context does not matter.** A test, a CLI, a script. Not in production handlers, because it ignores the request context — actor, deadline, tracing.

If you have a `*gin.Context` and want the body only (no context):

```go
reqval.Bind(c.Request.Body, &req)
```

That is useful when you are certain `Authorize` is a no-op and you do not need the deadline, but it is rarely what you want. Prefer `BindGin`.

## A complete Gin handler

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/thomas666-beast/reqval"
    "<your-module>/requests"
)

func CreatePost(c *gin.Context) {
    var req requests.StorePostRequest
    if err := reqval.BindGin(c, &req); err != nil {
        reqval.AbortWithError(c, err)
        return
    }
    c.JSON(201, gin.H{"title": req.Title, "body": req.Body})
}
```

`AbortWithError` inspects the error type and writes:

| Error type | Status | Body |
| :--- | :--- | :--- |
| `*AuthorizationError` | `403` | `{"error": "..."}` |
| `*ValidationError` | `422` | `{"errors": {...}}` |
| anything else (JSON decode, empty body, I/O) | `400` | `{"error": "..."}` |

## A complete `net/http` handler

```go
package main

import (
    "encoding/json"
    "net/http"
    "github.com/thomas666-beast/reqval"
    "<your-module>/requests"
)

func CreatePost(w http.ResponseWriter, r *http.Request) {
    var req requests.StorePostRequest
    if err := reqval.BindCtx(r.Context(), r.Body, &req); err != nil {
        if ve, ok := reqval.AsValidationError(err); ok {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(422)
            json.NewEncoder(w).Encode(map[string]any{"errors": ve.Fields})
            return
        }
        if ae, ok := reqval.AsAuthorizationError(err); ok {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(403)
            json.NewEncoder(w).Encode(map[string]any{"error": ae.Message})
            return
        }
        http.Error(w, err.Error(), 400)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(201)
    json.NewEncoder(w).Encode(map[string]string{"title": req.Title})
}
```

The three-branch logic is what `AbortWithError` automates for Gin. With `net/http`, you write it once and reuse it, or you write a small helper function.

## Handling errors yourself

`AbortWithError` is convenient, not mandatory. To render `ValidationError` differently — for example, as flash messages on a redirect:

```go
func CreatePost(c *gin.Context) {
    var req requests.StorePostRequest
    if err := reqval.BindGin(c, &req); err != nil {
        if ve, ok := reqval.AsValidationError(err); ok {
            // Store the errors in the session and redirect back to the form.
            c.SetCookie("errors", encode(ve.Fields), 300, "/", "", false, true)
            c.Redirect(http.StatusSeeOther, "/posts/create")
            return
        }
        reqval.AbortWithError(c, err)
        return
    }
    // ...
}
```

The point: the bind function returns the structured error; you decide what to do with it.

## What the request file looks like

The handler is not the only thing that matters. The request file defines the contract:

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

This is the file that sees all the action. The handler is a thin shell.

## Decoding rules

The decoder is `encoding/json` from the standard library. That means:

- Field names match by struct tag (`json:"title"`) or, if no tag, by Go field name (case-insensitive).
- Unknown fields in the body are ignored by default. To reject unknown fields, you have to decode first and re-marshal — `reqval` does not support strict mode.
- Numbers are decoded into `int`, `int64`, `float64`, etc. according to the field type. A JSON number that does not fit the target type produces a decode error (400), not a validation error (422).
- `json:"-"` excludes a field entirely.
- `null` sets a pointer field to `nil`; for a non-pointer field, it sets the zero value.

If you need to distinguish "not sent" from "sent as empty," use a pointer:

```go
type StorePostRequest struct {
    reqval.Base
    Title *string `json:"title"`
}
```

Then `req.Title == nil` means "not sent," `req.Title != nil && *req.Title == ""` means "sent, empty." Use `required` on the pointer field — `isEmpty` handles nil pointers via `reflect.ValueOf(v).IsZero()`.

## Testing a binding

The library's own test file tests `BindCtx` directly:

```go
func TestAuthorizeRunsBeforeDecode(t *testing.T) {
    err := reqval.BindCtx(context.Background(), strings.NewReader(`{bad json`), &authFailReq{})
    ae, ok := reqval.AsAuthorizationError(err)
    if !ok || ae.HTTPStatus() != 403 {
        t.Fatalf("want 403 before decode, got %v", err)
    }
}
```

The pattern is the same as your tests: build an `io.Reader` (or an `*http.Request`), call the bind function, inspect the error.

For a full HTTP test, use `httptest`:

```go
func TestCreatePost(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.POST("/posts", CreatePost)

    body := strings.NewReader(`{"title":"Hello","body":"This is long enough"}`)
    w := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodPost, "/posts", body)
    req.Header.Set("Content-Type", "application/json")
    r.ServeHTTP(w, req)

    if w.Code != 201 {
        t.Fatalf("want 201, got %d: %s", w.Code, w.Body.String())
    }
}
```

## Errors you will see

### `ErrEmptyBody`

Returned when the body is empty (zero bytes). Status 400. Handle it like any other decode error, or reject earlier with a middleware check.

### `*ValidationError`

`Fields` is `map[string]string`. Iterate it to render the JSON:

```go
for field, msg := range ve.Fields {
    fmt.Printf("%s: %s\n", field, msg)
}
```

### `*AuthorizationError`

`Message` is the error string from `Authorize`. Status 403.

### `json.Unmarshal` error

The raw standard library error. Status 400. `AbortWithError` renders it as `{"error": "<message>"}`.

## What to read next

- **[06-binding-form.md](06-binding-form.md)** — form-urlencoded bodies.
- **[07-file-uploads.md](07-file-uploads.md)** — multipart with files.
- **[10-error-handling.md](10-error-handling.md)** — more on rendering.

## Summary

Use `BindGin` in Gin, `BindCtx` in `net/http`, `Bind` only in tests or scripts. All three authorize → decode → validate in that order. `AbortWithError` writes the correct status for each error type. If you want a different response shape, inspect `*ValidationError` and `*AuthorizationError` yourself. The request file defines what makes a valid request; the handler is a thin shell.
