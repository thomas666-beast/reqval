# Authorization

`reqval` runs `Authorize(ctx)` **before** decoding the body. This file explains why, how to wire an actor into the context, what the 403 response looks like, and the one limitation that trips people up.

## Why before decode

Consider a non-admin who sends malformed JSON:

```
POST /admin/users
{not json
```

Two designs:

- **Decode first, then authorize.** The decoder fails with a JSON error. The client gets `400`. The client has now learned that their JSON was malformed — but they were never allowed to submit, so they should not get any information about the body at all.
- **Authorize first, then decode.** `Authorize` fails. The client gets `403`. The body is never parsed. Nothing is leaked.

`reqval` does the second. It matches Laravel's `FormRequest::authorize()`, which also runs before `rules()`.

The consequence: inside `Authorize`, the request struct is still zero-valued. `r.Title`, `r.ID`, and every other field is empty. This matters — see the caveat at the bottom.

## The method

```go
func (r *StorePostRequest) Authorize(ctx context.Context) error
```

Return `nil` to allow, any error to deny. The error's `Error()` string becomes the 403 body's `error` field.

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

The denial wraps in `*AuthorizationError` (HTTP 403). You do not construct that type yourself; returning an ordinary `error` from `Authorize` is enough.

## The actor comes from the context

`reqval` does not invent an actor model. You put the actor into the request's `context.Context`, and `Authorize` reads it back. That is the only integration point.

### Step 1 — define an actor type

Your application decides what an actor is. A common shape:

```go
package auth

type Actor struct {
    ID    int64
    Email string
    Role  string
}
```

### Step 2 — context key and helpers

A small set of helpers, in the same package:

```go
package auth

import "context"

type ctxKey int

const actorKey ctxKey = 1

func WithActor(ctx context.Context, a Actor) context.Context {
    return context.WithValue(ctx, actorKey, a)
}

func ActorFrom(ctx context.Context) (Actor, bool) {
    a, ok := ctx.Value(actorKey).(Actor)
    return a, ok
}
```

The `ctxKey` type is a private struct so no other package can collide with your key. This is the standard Go pattern for context values.

### Step 3 — middleware fills it in

A middleware reads your session or token and attaches the actor before the handler runs:

```go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Your own logic — session lookup, JWT decode, etc.
        actor := auth.Actor{
            ID:    42,
            Email: "alice@example.com",
            Role:  "admin",
        }
        ctx := auth.WithActor(c.Request.Context(), actor)
        c.Request = c.Request.WithContext(ctx)
        c.Next()
    }
}
```

Register it on the routes that need an actor:

```go
r := gin.Default()
r.Use(AuthMiddleware())
r.POST("/posts", CreatePost)
```

### Step 4 — `Authorize` reads it

```go
func (r *StorePostRequest) Authorize(ctx context.Context) error {
    actor, ok := auth.ActorFrom(ctx)
    if !ok {
        return errors.New("unauthenticated")
    }
    if actor.Role != "admin" {
        return errors.New("admin only")
    }
    return nil
}
```

`BindGin` passes `c.Request.Context()` to `Authorize`, so the actor is available.

For `net/http`, `r.Context()` is passed by `BindCtx`, so the same pattern works:

```go
if err := reqval.BindCtx(r.Context(), r.Body, &req); err != nil {
    // ...
}
```

## What the client sees

### Denied

```bash
curl -X POST http://localhost:8080/admin/users \
  -H 'Content-Type: application/json' \
  -d '{"name":"bob"}'
# 403 {"error":"admin only"}
```

The `error` field is the string from `Authorize`'s error. Change the wording to change what clients see.

### Allowed, but invalid body

```bash
curl -X POST http://localhost:8080/admin/users \
  -H 'Authorization: Bearer admin-token' \
  -H 'Content-Type: application/json' \
  -d '{"name":""}'
# 422 {"errors":{"Name":"The Name field is invalid."}}
```

Notice: the 403 does not appear when the actor is an admin. `Authorize` returned `nil`, so decoding and validation ran normally.

### Allowed, malformed body

```bash
curl -X POST http://localhost:8080/admin/users \
  -H 'Authorization: Bearer admin-token' \
  -H 'Content-Type: application/json' \
  -d '{not json'
# 400 {"error":"invalid character 'o' looking for beginning of object key string"}
```

Once authorization passes, a malformed body is a 400. Before it passes, a malformed body is a 403 with no body parsed.

## Multiple authorization shapes

### Admin only

```go
func (r *AdminRequest) Authorize(ctx context.Context) error {
    a, ok := auth.ActorFrom(ctx)
    if !ok || a.Role != "admin" {
        return errors.New("admin only")
    }
    return nil
}
```

### Owner of the resource

```go
func (r *UpdatePostRequest) Authorize(ctx context.Context) error {
    a, ok := auth.ActorFrom(ctx)
    if !ok {
        return errors.New("unauthenticated")
    }
    // The post ID is in the URL, not the request body.
    postID := ctx.Value("post_id").(int64)
    owner, err := loadPostOwner(ctx, postID)
    if err != nil {
        return errors.New("not found")
    }
    if a.ID != owner {
        return errors.New("can only edit your own posts")
    }
    return nil
}
```

The `ctx.Value("post_id")` line is illustrative — your middleware would put the route param into the context or into a typed struct. The point is: `Authorize` can look up anything it needs from the context or from outside the request body.

### Authenticated, any role

```go
func (r *CreatePostRequest) Authorize(ctx context.Context) error {
    if _, ok := auth.ActorFrom(ctx); !ok {
        return errors.New("unauthenticated")
    }
    return nil
}
```

### Public, no auth

Omit the method. `reqval.Base` provides a default that returns `nil`:

```go
type PublicRequest struct {
    reqval.Base
    // ...
}

func (r *PublicRequest) Rules() map[string][]string {
    return map[string][]string{"Title": {"required"}}
}
```

Or, if you want it explicit:

```go
func (r *PublicRequest) Authorize(ctx context.Context) error {
    return nil
}
```

## The caveat: `Authorize` runs before decode

`Authorize` receives `ctx`, but the request struct is still zero-valued. `r.ID`, `r.Title`, everything is empty.

This is the point of the ordering — it prevents unauthorized callers from learning whether their body parses. But it means you cannot use body fields inside `Authorize`.

### What to do instead

If authorization depends on a field from the body, that field needs to come from somewhere else first:

**Option A — use a path or query parameter.** For "can this user edit post 42?", the post ID belongs in the URL, not the body. Extract it in the handler before calling `BindGin`, and attach it to the context:

```go
func UpdatePost(c *gin.Context) {
    postID, _ := strconv.ParseInt(c.Param("id"), 10, 64)
    ctx := context.WithValue(c.Request.Context(), "post_id", postID)
    c.Request = c.Request.WithContext(ctx)

    var req UpdatePostRequest
    if err := reqval.BindGin(c, &req); err != nil {
        reqval.AbortWithError(c, err)
        return
    }
    // ...
}
```

Now `Authorize` can read `ctx.Value("post_id")` and load the post to check ownership.

**Option B — decode first, authorize in the handler.** For requests where the body genuinely determines authorization and path parameters are not available, do the decode yourself before calling `Authorize`:

```go
func UpdatePost(c *gin.Context) {
    var req UpdatePostRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        reqval.AbortWithError(c, err)
        return
    }
    if req.Author != actor.ID {
        reqval.AbortWithError(c, &reqval.AuthorizationError{Message: "not your post"})
        return
    }
    if err := reqval.Validate(&req); err != nil {
        reqval.AbortWithError(c, err)
        return
    }
    // ...
}
```

That sacrifices the "no decode before auth" property for that one request. It is the right trade when the body is what determines authorization.

**Option C — split the request.** Put the auth-relevant fields in the URL or in a query param, the rest in the body. That is usually the cleanest design.

## Rendering the 403

`AbortWithError` writes:

```json
{"error":"admin only"}
```

with status 403. If you want a different shape (e.g. a problem-details document), inspect the error yourself:

```go
if err := reqval.BindGin(c, &req); err != nil {
    if ae, ok := reqval.AsAuthorizationError(err); ok {
        c.JSON(403, gin.H{
            "status":  "forbidden",
            "message": ae.Message,
        })
        return
    }
    // ...
}
```

## Testing authorization

Construct a context with the actor you want and call `BindCtx`:

```go
func TestAdminOnly(t *testing.T) {
    ctx := auth.WithActor(context.Background(), auth.Actor{Role: "editor"})
    req := &StorePostRequest{}
    err := reqval.BindCtx(ctx, strings.NewReader(`{"title":"x"}`), req)
    ae, ok := reqval.AsAuthorizationError(err)
    if !ok || ae.HTTPStatus() != 403 {
        t.Fatalf("want 403, got %v", err)
    }

    ctx = auth.WithActor(context.Background(), auth.Actor{Role: "admin"})
    if err := reqval.BindCtx(ctx, strings.NewReader(`{"title":"x"}`), req); err != nil {
        t.Fatalf("admin should pass: %v", err)
    }
}
```

The library's own test file does exactly this for the ordering guarantee (that a non-admin with malformed JSON gets 403, not 400).

## What to read next

- **[05-binding-json.md](05-binding-json.md)** — how `BindGin` / `BindCtx` call `Authorize`.
- **[10-error-handling.md](10-error-handling.md)** — rendering `AuthorizationError` differently from `ValidationError`.

## Summary

`Authorize(ctx)` runs before decode. Return `nil` to allow, error to deny. Denial becomes `*AuthorizationError` with HTTP 403. The actor comes from the request context — put it there in middleware. Body fields are zero inside `Authorize`; if you need a field, it has to come from the URL or a query param, attached to the context before binding.
