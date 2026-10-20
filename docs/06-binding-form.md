# Binding form data

Three functions bind an `application/x-www-form-urlencoded` body to a request struct: `BindForm`, `BindFormCtx`, and `BindFormGin`. They are the form-body equivalents of `Bind`, `BindCtx`, and `BindGin`.

This file explains how they differ from the JSON path, how the form decoder works, and the one limitation that matters.

## The three functions

| Function | Signature | Input | Context |
| :--- | :--- | :--- | :--- |
| `BindForm` | `BindForm(r io.Reader, contentType string, dst any) error` | An `io.Reader` plus the content type | `context.Background()` |
| `BindFormCtx` | `BindFormCtx(ctx context.Context, r *http.Request, dst any) error` | An `*http.Request` | Your context |
| `BindFormGin` | `BindFormGin(c *gin.Context, dst any) error` | A Gin context | `c.Request.Context()` |

All three do the same thing:

1. If `dst` implements `reqval.Authorizer`, call `Authorize(ctx)`. On error, return `*AuthorizationError`.
2. Check `Content-Type`. It must be `application/x-www-form-urlencoded`. Anything else returns an error (400).
3. Parse the form body into `r.PostForm` (or `r.MultipartForm` for multipart).
4. Decode the form values into the struct using `form:` tags (falling back to `json:` tags).
5. Validate with `ValidateCtx`.

`BindFormCtx` and `BindFormGin` take the request directly, so they can parse the body. `BindForm` takes an `io.Reader` and a content-type string; it wraps them in a temporary `*http.Request` internally, which is useful in tests but cumbersome in production handlers.

Prefer `BindFormGin` in Gin, `BindFormCtx` in `net/http`.

## A complete Gin handler

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/thomas666-beast/reqval"
    "<your-module>/requests"
)

func Login(c *gin.Context) {
    var req requests.LoginRequest
    if err := reqval.BindFormGin(c, &req); err != nil {
        reqval.AbortWithError(c, err)
        return
    }
    c.JSON(200, gin.H{"email": req.Email})
}
```

The handler is the same shape as the JSON case. `AbortWithError` writes:

| Error type | Status | Body |
| :--- | :--- | :--- |
| `*AuthorizationError` | `403` | `{"error": "..."}` |
| `*ValidationError` | `422` | `{"errors": {...}}` |
| wrong content type or parse error | `400` | `{"error": "..."}` |

## A complete `net/http` handler

```go
package main

import (
    "encoding/json"
    "net/http"
    "github.com/thomas666-beast/reqval"
    "<your-module>/requests"
)

func Login(w http.ResponseWriter, r *http.Request) {
    var req requests.LoginRequest
    if err := reqval.BindFormCtx(r.Context(), r, &req); err != nil {
        if ve, ok := reqval.AsValidationError(err); ok {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(422)
            json.NewEncoder(w).Encode(map[string]any{"errors": ve.Fields})
            return
        }
        http.Error(w, err.Error(), 400)
        return
    }
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{"email": req.Email})
}
```

## The request file

The request file looks almost like the JSON one. The difference is the struct tag: `form:` instead of `json:`.

```go
package requests

import (
    "context"
    "github.com/thomas666-beast/reqval"
)

type LoginRequest struct {
    reqval.Base
    Email    string   `form:"email"`
    Password string   `form:"password"`
    Roles    []string `form:"roles"`
}

func (r *LoginRequest) Rules() map[string][]string {
    return map[string][]string{
        "Email":    {"required", "email"},
        "Password": {"required", "min:8"},
        "Roles":    {"required", "min:1"},
    }
}

func (r *LoginRequest) Messages() map[string]string {
    return map[string]string{
        "Email.email": "That is not a valid email.",
    }
}

func (r *LoginRequest) Authorize(ctx context.Context) error {
    return nil
}
```

`Rules()`, `Messages()`, and `Authorize()` are identical to the JSON version. Only the struct tags differ. That is by design — the same request can serve JSON or form clients, you just swap which bind function the handler calls.

## How the form decoder works

The decoder walks the struct fields and looks up each one in the parsed form values. The tag lookup is:

1. Use `form:"name"` if present.
2. Otherwise use `json:"name"` if present.
3. Otherwise use the Go field name.

Supported field kinds:

| Kind | How the string is decoded |
| :--- | :--- |
| `string` | assigned as-is |
| `int`, `int8`, `int16`, `int32`, `int64` | `strconv.ParseInt` |
| `uint`, `uint8`, `uint16`, `uint32`, `uint64` | `strconv.ParseUint` |
| `float32`, `float64` | `strconv.ParseFloat` |
| `bool` | `strconv.ParseBool` |
| `[]string` | assigned all values for that key |

Anything else (structs, maps, pointers, slices of non-strings) is not decoded and is left at its zero value. If your request has a field of an unsupported type, the form decoder silently skips it. Use `required` if the field must be populated — the rule will fail, and you will see the error.

### `[]string` and repeated keys

The form decoder assigns **all** values for a key to a `[]string` field. A form body like:

```
roles=admin&roles=editor&email=a%40b.com
```

produces:

```go
req.Roles == []string{"admin", "editor"}
req.Email == "a@b.com"
```

This is how form clients send arrays. The `url.Values` type represents multi-valued keys natively.

### Empty values

A field that appears in the form with an empty value (`email=`) is decoded as `""`. A field that does not appear at all keeps its zero value. There is no difference for strings — both are `""` — but for integers, a missing field stays `0` while `age=0` also becomes `0`. If you need to distinguish "not sent" from "sent as 0" or "sent as empty," use a pointer field (`*string`, `*int`); the decoder does not currently support pointers, so this is not possible without a custom decode step.

In practice this matters less than it sounds. `required` on a string rejects both `""` (not sent) and `""` (sent empty), which is what you want for required fields. `min:1` on a slice counts elements, which is what you want for required arrays.

## Wrong content type

If the `Content-Type` header is not `application/x-www-form-urlencoded`, `BindFormCtx` returns an error:

```
reqval: expected application/x-www-form-urlencoded, got application/json
```

Status 400. That is deliberate: a handler that expects form data should reject JSON, not try to parse it. If you want a handler that accepts both, use a dispatcher:

```go
func Create(c *gin.Context) {
    ct := c.GetHeader("Content-Type")
    var req CreateRequest
    var err error
    if strings.HasPrefix(ct, "application/json") {
        err = reqval.BindGin(c, &req)
    } else if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
        err = reqval.BindFormGin(c, &req)
    } else {
        c.AbortWithStatusJSON(415, gin.H{"error": "unsupported media type"})
        return
    }
    if err != nil {
        reqval.AbortWithError(c, err)
        return
    }
    // ...
}
```

The `CreateRequest` struct then needs both `json:` and `form:` tags on the same fields:

```go
type CreateRequest struct {
    reqval.Base
    Name  string `json:"name"  form:"name"`
    Email string `json:"email" form:"email"`
}
```

That is a perfectly reasonable pattern. The form decoder tries `form:` first, and the JSON decoder ignores `form:` entirely.

## Testing a form binding

```go
func TestLogin(t *testing.T) {
    form := url.Values{
        "email":    {"a@b.com"},
        "password": {"hunter2hunter"},
        "roles":    {"admin", "editor"},
    }
    body := strings.NewReader(form.Encode())
    req := httptest.NewRequest(http.MethodPost, "/login", body)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    var dst LoginRequest
    if err := reqval.BindFormCtx(context.Background(), req, &dst); err != nil {
        t.Fatalf("want valid form, got %v", err)
    }
    if dst.Email != "a@b.com" {
        t.Fatalf("want email a@b.com, got %q", dst.Email)
    }
    if len(dst.Roles) != 2 || dst.Roles[0] != "admin" {
        t.Fatalf("want roles [admin editor], got %v", dst.Roles)
    }
}
```

For an invalid case, omit a required field and assert the `Fields` map:

```go
func TestLoginInvalid(t *testing.T) {
    form := url.Values{"email": {"nope"}, "password": {"x"}}
    body := strings.NewReader(form.Encode())
    req := httptest.NewRequest(http.MethodPost, "/login", body)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    var dst LoginRequest
    err := reqval.BindFormCtx(context.Background(), req, &dst)
    ve, ok := reqval.AsValidationError(err)
    if !ok {
        t.Fatalf("want validation error, got %v", err)
    }
    for _, f := range []string{"Email", "Password", "Roles"} {
        if _, has := ve.Fields[f]; !has {
            t.Fatalf("want %s flagged, got %v", f, ve.Fields)
        }
    }
}
```

## What to read next

- **[07-file-uploads.md](07-file-uploads.md)** — `multipart/form-data` with files.
- **[05-binding-json.md](05-binding-json.md)** — the JSON path, for comparison.
- **[02-rules.md](02-rules.md)** — the `min`/`max` behavior on slices (the `roles` example depends on it).

## Summary

Use `BindFormGin` in Gin, `BindFormCtx` in `net/http`. The request file is the same as JSON, with `form:` tags instead of `json:` tags. Content type must be `application/x-www-form-urlencoded`. `[]string` fields accept repeated keys. Wrong content type returns 400. `Rules()`, `Messages()`, `Authorize()` are identical to the JSON case.
