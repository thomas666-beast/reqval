# Error handling

`reqval` returns three kinds of error. This file explains each, when it occurs, how to discriminate, and how to render it — including the cases where `AbortWithError` is not the shape you want.

## The three error types

| Type | HTTP | Unwrap with | When |
| :--- | :--- | :--- | :--- |
| `*AuthorizationError` | `403` | `reqval.AsAuthorizationError(err)` | `Authorize` returned non-nil |
| `*ValidationError` | `422` | `reqval.AsValidationError(err)` | A rule failed |
| other `error` | `400` | plain `err` | JSON decode failure, empty body, wrong content type, I/O error |

Every bind function (`Bind`, `BindCtx`, `BindGin`, `BindForm*`, `BindMultipart*`) returns one of these. Every `Validate` / `ValidateCtx` call returns `nil` or `*ValidationError`.

## `*ValidationError`

```go
type ValidationError struct {
    Fields map[string]string // field name or path -> message
}

func (e *ValidationError) Error() string       // for logging
func (e *ValidationError) HTTPStatus() int     // always 422
```

`Fields` is the payload. Keys are field names (or paths for nested rules, e.g. `Items.1.Name`). Values are the resolved messages.

### Reading it

```go
if err := reqval.BindGin(c, &req); err != nil {
    if ve, ok := reqval.AsValidationError(err); ok {
        for field, msg := range ve.Fields {
            log.Printf("%s: %s", field, msg)
        }
        // ...
    }
}
```

The map iteration order is not deterministic. If you need stable output (a test, a diff), sort the keys:

```go
keys := make([]string, 0, len(ve.Fields))
for k := range ve.Fields {
    keys = append(keys, k)
}
sort.Strings(keys)
for _, k := range keys {
    fmt.Printf("%s: %s\n", k, ve.Fields[k])
}
```

### The `Error()` method

`ve.Error()` returns a summary string for logging:

```
validation failed on 3 field(s)
```

It does not include the field names or messages. That is deliberate: `Error()` is for logs and stack traces, not for the client. Use `Fields` for the client payload.

### `HTTPStatus()`

Always returns `422`. The method exists so that a generic error-to-status mapper can call it without type-switching:

```go
type statusCoder interface {
    HTTPStatus() int
}

func statusFor(err error) int {
    if sc, ok := err.(statusCoder); ok {
        return sc.HTTPStatus()
    }
    return 400
}
```

Both `ValidationError` and `AuthorizationError` satisfy that interface. Anything else falls back to 400.

## `*AuthorizationError`

```go
type AuthorizationError struct {
    Message string
}

func (e *AuthorizationError) Error() string       // the Message, or "forbidden"
func (e *AuthorizationError) HTTPStatus() int     // always 403
```

`Message` is the error string returned by `Authorize`. It is safe to send to the client — that is why you control the wording.

### Reading it

```go
if ae, ok := reqval.AsAuthorizationError(err); ok {
    c.JSON(403, gin.H{"error": ae.Message})
    return
}
```

## Other errors (status 400)

Everything else is a client error with a message from the underlying failure:

- **Empty body** — `reqval.ErrEmptyBody` (`"reqval: empty request body"`).
- **JSON decode failure** — the raw `json.Unmarshal` error, e.g. `"invalid character 'o' looking for beginning of object key string"`.
- **Wrong content type** — `"reqval: expected application/x-www-form-urlencoded, got application/json"`.
- **Multipart parse failure** — the raw `multipart.Reader` error.

These are not typed by `reqval`; they are plain `error` values. Treat them as 400 by default, and do not leak their contents to the client if they might reveal internal state. In practice, the messages above are safe — they are the parser's own messages, not stack traces.

To detect an empty body specifically:

```go
if errors.Is(err, reqval.ErrEmptyBody) {
    // 400, "no body"
}
```

## Discriminating in a handler

The full pattern:

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

That is exactly what `reqval.AbortWithError(c, err)` does. Use the one-liner if you like the shape; use the explicit version if you want to log differently or add a request ID:

```go
if err := reqval.BindGin(c, &req); err != nil {
    if ve, ok := reqval.AsValidationError(err); ok {
        log.Printf("validation failed: %v", ve.Fields)
        c.JSON(422, gin.H{"errors": ve.Fields, "request_id": requestID(c)})
        return
    }
    if ae, ok := reqval.AsAuthorizationError(err); ok {
        log.Printf("authz denied: %s", ae.Message)
        c.JSON(403, gin.H{"error": ae.Message})
        return
    }
    log.Printf("bad request: %v", err)
    c.JSON(400, gin.H{"error": "bad request"}) // generic message
    return
}
```

Note the last branch: unlike `AbortWithError`, this version does **not** send `err.Error()` to the client. A JSON decode error message can include fragments of the body, which is sometimes sensitive. Sending a generic "bad request" is a valid choice.

## Flash, redirect, HTML

`AbortWithError` writes JSON. If you want a different response medium, `reqval` does not impose one. The `*ValidationError.Fields` map is data; how you deliver it is up to you.

### Flash on the session

```go
func Create(c *gin.Context) {
    var req CreateRequest
    if err := reqval.BindGin(c, &req); err != nil {
        if ve, ok := reqval.AsValidationError(err); ok {
            c.SetCookie("errors", encodeErrors(ve.Fields), 300, "/", "", false, true)
            c.Redirect(303, "/posts/create")
            return
        }
        reqval.AbortWithError(c, err)
        return
    }
    // ...
}

func encodeErrors(fields map[string]string) string {
    // JSON, base64, or a session cookie — your choice.
    b, _ := json.Marshal(fields)
    return base64.URLEncoding.EncodeToString(b)
}
```

### Render HTML

```go
if ve, ok := reqval.AsValidationError(err); ok {
    c.HTML(422, "form.html", gin.H{
        "errors": ve.Fields,
        "form":   req,
    })
    return
}
```

In the template:

```html
{{ if .errors }}
<ul class="errors">
  {{ range $field, $msg := .errors }}
  <li>{{ $field }}: {{ $msg }}</li>
  {{ end }}
</ul>
{{ end }}
```

### Return the errors as a top-level array

Some APIs prefer `{"errors": ["msg1", "msg2"]}` over an object. Flatten:

```go
msgs := make([]string, 0, len(ve.Fields))
for _, m := range ve.Fields {
    msgs = append(msgs, m)
}
c.JSON(422, gin.H{"errors": msgs})
```

`reqval` does not care which shape you choose. The `Fields` map is the source of truth; you translate it.

## Middleware-level handling

If every route uses the same handler shape, factor the error rendering into middleware:

```go
func ValidationMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Next()
        if len(c.Errors) == 0 {
            return
        }
        err := c.Errors.Last().Err
        if ae, ok := reqval.AsAuthorizationError(err); ok {
            c.JSON(403, gin.H{"error": ae.Message})
            c.Abort()
            return
        }
        if ve, ok := reqval.AsValidationError(err); ok {
            c.JSON(422, gin.H{"errors": ve.Fields})
            c.Abort()
            return
        }
    }
}
```

Then in the handler:

```go
if err := reqval.BindGin(c, &req); err != nil {
    _ = c.Error(err)
    return
}
```

That is a valid alternative when you have many handlers. `reqval` does not require it.

## Logging

A pattern that logs the structured error but returns a clean response:

```go
if err := reqval.BindGin(c, &req); err != nil {
    if ve, ok := reqval.AsValidationError(err); ok {
        log.Printf("validation_error fields=%v", ve.Fields)
        c.JSON(422, gin.H{"errors": ve.Fields})
        return
    }
    if ae, ok := reqval.AsAuthorizationError(err); ok {
        log.Printf("authz_error msg=%q", ae.Message)
        c.JSON(403, gin.H{"error": ae.Message})
        return
    }
    log.Printf("bad_request err=%v", err)
    c.JSON(400, gin.H{"error": "bad request"})
    return
}
```

The logged form is verbose enough for post-mortems (`fields=map[Title:... Body:...]`) without sending stack traces to the client.

## What the client sees

For the same request, the three outcomes:

```bash
# valid
curl -X POST http://localhost:8080/posts -d '{"title":"x","body":"long enough"}'
# 201 {"slug":"x"}

# validation failure
curl -X POST http://localhost:8080/posts -d '{"title":"","body":"short"}'
# 422 {"errors":{"Body":"The Body field is invalid.","Title":"The Title field is invalid."}}

# authorization failure
curl -X POST http://localhost:8080/admin/posts -d '{"title":"x","body":"long enough"}'
# 403 {"error":"admin only"}

# malformed body
curl -X POST http://localhost:8080/posts -d '{oops'
# 400 {"error":"invalid character 'o' looking for beginning of object key string"}
```

The four statuses (201/422/403/400) cover the entire space. There is no 500 path from `reqval` itself; a 500 comes from your handler code, not from validation.

## Testing the error shapes

```go
func TestValidationError(t *testing.T) {
    err := reqval.ValidateCtx(context.Background(), &storePostRequest{})
    ve, ok := reqval.AsValidationError(err)
    if !ok {
        t.Fatalf("want validation error, got %v", err)
    }
    if len(ve.Fields) == 0 {
        t.Fatal("want at least one field error")
    }
    if ve.HTTPStatus() != 422 {
        t.Fatalf("want 422, got %d", ve.HTTPStatus())
    }
}

func TestAuthorizationError(t *testing.T) {
    err := reqval.BindCtx(context.Background(), strings.NewReader(`{}`), &authFailReq{})
    ae, ok := reqval.AsAuthorizationError(err)
    if !ok {
        t.Fatalf("want authz error, got %v", err)
    }
    if ae.HTTPStatus() != 403 {
        t.Fatalf("want 403, got %d", ae.HTTPStatus())
    }
}
```

`AsValidationError` and `AsAuthorizationError` are the canonical discriminators. They use `errors.As` internally, so wrapped errors work too:

```go
wrapped := fmt.Errorf("handler failed: %w", err)
ve, ok := reqval.AsValidationError(wrapped) // works
```

That matters if you wrap errors in your own code before returning them.

## What to read next

- **[03-messages.md](03-messages.md)** — how the messages inside `Fields` are resolved.
- **[04-authorization.md](04-authorization.md)** — the `AuthorizationError` path, in depth.
- **[05-binding-json.md](05-binding-json.md)** — where the errors come from.

## Summary

Three error types: `*AuthorizationError` (403), `*ValidationError` (422), plain errors (400). Discriminate with `AsAuthorizationError` and `AsValidationError`. `ValidationError.Fields` is `map[string]string`; render it however you like. `AbortWithError` is the one-liner for JSON; for flash, HTML, or custom shapes, inspect the error and write your own response. `HTTPStatus()` on both types lets a generic mapper work without type-switching.
