# Getting started

This file takes you from zero to a working request in `reqval`. By the end you will have:

- Installed the library.
- Written your first request file.
- Wired it into a Gin handler.
- Called the endpoint with `curl`.
- Written a test that proves it works.

Every example is runnable. Nothing is stubbed.

## 1. Install

```bash
go get github.com/thomas666-beast/reqval
```

If you do not already have Gin:

```bash
go get github.com/gin-gonic/gin
```

Requires Go 1.22 or newer.

## 2. Your first request file

Create `requests/store_post_request.go` in your project:

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
    return nil // no auth check for this endpoint
}
```

That is the whole request file. Three things to notice:

1. `reqval.Base` is embedded. It satisfies the `Request` interface with empty defaults, so you only write the methods you need. (You wrote all three here because the example shows all three; in real code you would omit `Authorize` if it returned `nil` and omit `Messages` if there were no custom messages.)

2. `Rules()` returns a map from field name to a slice of rule strings. The order matters: rules run left-to-right and stop at the first failure.

3. The struct fields carry `json:` tags so the decoder fills them from the body. They do **not** carry `validate:` tags. Rules live in the method, not in tags.

## 3. Wire it into a handler

Create `main.go`:

```go
package main

import (
    "github.com/gin-gonic/gin"
    "github.com/thomas666-beast/reqval"
    "<your-module>/requests"
)

func main() {
    r := gin.Default()

    r.POST("/posts", func(c *gin.Context) {
        var req requests.StorePostRequest
        if err := reqval.BindGin(c, &req); err != nil {
            reqval.AbortWithError(c, err)
            return
        }
        c.JSON(201, gin.H{"title": req.Title, "body": req.Body})
    })

    r.Run(":8080")
}
```

Replace `<your-module>` with the module name from your `go.mod` (e.g. `github.com/yourname/blog`).

The handler does three things:

1. Declares a `StorePostRequest`.
2. Calls `reqval.BindGin`, which authorizes, decodes, and validates — in that order.
3. If `err` is non-nil, `AbortWithError` writes the correct status and body and returns. If `err` is nil, the request is valid and the handler proceeds.

The handler never mentions `req.Title == ""` again.

## 4. Call it

```bash
go run .
```

In another terminal:

```bash
# valid
curl -X POST http://localhost:8080/posts \
  -H 'Content-Type: application/json' \
  -d '{"title":"Hello","body":"This is long enough"}'
# 201 {"body":"This is long enough","title":"Hello"}

# invalid — empty title, short body
curl -X POST http://localhost:8080/posts \
  -H 'Content-Type: application/json' \
  -d '{"title":"","body":"short"}'
# 422 {"errors":{"Body":"The Body field is invalid.","Title":"We really need a title."}}

# malformed JSON
curl -X POST http://localhost:8080/posts \
  -H 'Content-Type: application/json' \
  -d '{oops'
# 400 {"error":"invalid character 'o' looking for beginning of object key string"}
```

Three input shapes, three consistent outputs:

| Input | Status | Body |
| :--- | :--- | :--- |
| Valid | 201 | The created resource |
| Invalid rules | 422 | `{"errors": {...}}` |
| Malformed body | 400 | `{"error": "..."}` |

Note the 422 in the second call: `Title` carries the custom message from `Messages()` ("We really need a title."), while `Body` uses the fallback ("The Body field is invalid."). That is because the request only overrode `Title.required`. If you want a specific message for `Body.min` too, add it:

```go
func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "Title.required": "We really need a title.",
        "Body.min":       "The body must be at least {{param}} characters.",
    }
}
```

Then the 422 body becomes:

```json
{"errors":{"Body":"The body must be at least 10 characters.","Title":"We really need a title."}}
```

`{{param}}` is replaced with the rule's parameter — here, `10`.

## 5. Write a test

Tests do not need the HTTP layer. Call `Validate` directly:

```go
package requests_test

import (
    "context"
    "testing"

    "github.com/thomas666-beast/reqval"
    "<your-module>/requests"
)

func TestStorePostRequestValid(t *testing.T) {
    req := &requests.StorePostRequest{
        Title: "Hello",
        Body:  "This is long enough",
    }
    if err := reqval.ValidateCtx(context.Background(), req); err != nil {
        t.Fatalf("want valid, got %v", err)
    }
}

func TestStorePostRequestMissingTitle(t *testing.T) {
    req := &requests.StorePostRequest{
        Body: "This is long enough",
    }
    err := reqval.ValidateCtx(context.Background(), req)
    ve, ok := reqval.AsValidationError(err)
    if !ok {
        t.Fatalf("want validation error, got %v", err)
    }
    if ve.Fields["Title"] != "We really need a title." {
        t.Fatalf("want custom message, got %q", ve.Fields["Title"])
    }
}
```

Run:

```bash
go test ./requests
```

Both tests pass. No database, no HTTP server, no fixtures. The request file is the test subject.

## 6. What to read next

- **[01-writing-a-request.md](01-writing-a-request.md)** — the anatomy of a request file, in detail.
- **[02-rules.md](02-rules.md)** — every rule, with input/output tables.
- **[10-error-handling.md](10-error-handling.md)** — when you want to render 422 differently (flash, redirect, HTML).

## Summary

The shape of every `reqval` request:

1. A struct embedding `reqval.Base`, with `json:` tags for decoding.
2. `Rules()` — map of field name to rule strings.
3. `Messages()` — optional, map of `<field>.<rule>` to template.
4. `Authorize(ctx)` — optional, return `nil` to allow.

The shape of every handler:

1. Declare the request struct.
2. Call `BindGin` (or `BindCtx` / `BindFormGin` / `BindMultipartGin`).
3. On error, call `AbortWithError` and return.
4. On success, use the fields.
