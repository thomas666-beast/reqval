# Writing a request

This file explains the anatomy of a request file: the struct, the three methods, and the design decisions behind them. By the end you will know exactly what every part does and why.

Every request file in `reqval` has the same shape:

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

func (r *StorePostRequest) Rules() map[string][]string        { ... }
func (r *StorePostRequest) Messages() map[string]string        { ... }
func (r *StorePostRequest) Authorize(ctx context.Context) error { ... }
```

Four parts. We will go through each one.

## 1. The struct

The struct holds the fields the form accepts. Every field carries a `json:` tag (or `form:` for form bodies, or no tag if you use the field name directly).

```go
type StorePostRequest struct {
    reqval.Base

    Title      string `json:"title"`
    Body       string `json:"body"`
    CategoryID int64  `json:"category_id"`
    Tags       []string `json:"tags"`
}
```

Notes on the field types:

- `string`, `int`, `int64`, `float64`, `bool` — the scalar types. Decoded directly from JSON, form, or multipart.
- `[]string` — decoded as an array of strings. In forms, appears as repeated keys (`tags=a&tags=b`). In multipart, same.
- A struct — decoded from a nested JSON object. In forms and multipart, not supported (there is no syntax for nested keys in the standard form encoding).
- A slice of a struct — decoded from a JSON array of objects. In forms and multipart, not supported.

If a field is not present in the body, it remains its zero value. `reqval` does not distinguish "not present" from "present but empty" by default. If you need that distinction, use `required` on the field and let the client send the empty value explicitly.

### The `reqval.Base` embed

`reqval.Base` is a zero-sized struct that provides the three interface methods with default implementations:

```go
type Base struct{}

func (Base) Rules() map[string][]string          { return nil }
func (Base) Messages() map[string]string          { return nil }
func (Base) Authorize(ctx context.Context) error  { return nil }
```

Embedding it means you can write only the methods you need. A request with no authorization and no custom messages just writes `Rules()`:

```go
type SimpleRequest struct {
    reqval.Base
    Name string `json:"name"`
}

func (r *SimpleRequest) Rules() map[string][]string {
    return map[string][]string{"Name": {"required"}}
}
```

That request satisfies `reqval.Request`. `reqval.Base` provides the other two.

## 2. `Rules()`

`Rules()` returns a map from field name (or path) to a slice of rule strings:

```go
func (r *StorePostRequest) Rules() map[string][]string {
    return map[string][]string{
        "Title":      {"required", "max:255"},
        "Body":       {"required", "min:10"},
        "CategoryID": {"required", "exists:categories,id"},
    }
}
```

The keys are **Go field names**, not JSON tag names. `"Title"`, not `"title"`. The reflection walk uses `FieldByName`, which is case-sensitive and looks for the exact field name in the struct.

Values are slices of rule strings. Order matters: rules run left-to-right and stop at the first failure.

### Rule syntax

Each rule is one of three forms:

```
"required"                   name only
"max:255"                    name:param
"between:1,100"              name:param1,param2
"unique:users,email,id,42"   name:param1,param2,param3,param4
```

The colon separates the name from the params. Commas separate params within a rule. See [02-rules.md](02-rules.md) for the full list.

### Why rules are in a method, not tags

Struct tags are static: they are computed at compile time and cannot depend on runtime values. Rules in a method can:

```go
func (r *UpdateUserRequest) Rules() map[string][]string {
    return map[string][]string{
        "Username": {"required", "unique:users,username,id," + strconv.FormatInt(r.ID, 10)},
    }
}
```

Here the fourth parameter of `unique` is the current row's ID, taken from the request itself. That is only possible with a method.

Laravel made the same choice: `rules()` is a method in `FormRequest`, not a property.

### Empty `Rules()`

An empty `Rules()` means "no validation". That is useful for pure-decoding requests:

```go
func (r *WebhookRequest) Rules() map[string][]string {
    return nil
}
```

The handler still gets the decoded struct; no rules fire.

## 3. `Messages()`

`Messages()` returns a map from `<field>.<rule>` to a message template:

```go
func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "Title.required": "We really need a title.",
        "Body.min":       "The body must be at least {{param}} characters.",
    }
}
```

The key is `<Field>.<rule>` — the Go field name, a dot, then the rule name. Placeholders `{{field}}` and `{{param}}` are replaced when the message is rendered.

If a request has no `Messages()` method, or returns `nil`, the application default bundle applies (see [03-messages.md](03-messages.md)). If neither the request nor the bundle defines a message, the fallback `"The <field> field is invalid."` is used.

## 4. `Authorize(ctx)`

`Authorize` runs **before** decoding. It returns `nil` to allow, any error to deny:

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

Denial wraps the error in `*AuthorizationError` (HTTP 403). The error's `Error()` method becomes the 403 body's `error` field.

See [04-authorization.md](04-authorization.md) for the full story, including how to put the actor into the context.

## Putting it together

A complete request file, no omissions:

```go
package requests

import (
    "context"
    "errors"
    "strconv"

    "github.com/thomas666-beast/reqval"
)

type UpdatePostRequest struct {
    reqval.Base

    ID    int64  `json:"id"`
    Title string `json:"title"`
    Body  string `json:"body"`
}

func (r *UpdatePostRequest) Rules() map[string][]string {
    return map[string][]string{
        "Title": {"required", "max:255"},
        "Body":  {"required", "min:10"},
    }
}

func (r *UpdatePostRequest) Messages() map[string]string {
    return map[string]string{
        "Title.required": "A title is required.",
    }
}

func (r *UpdatePostRequest) Authorize(ctx context.Context) error {
    actor, ok := ActorFrom(ctx)
    if !ok {
        return errors.New("unauthenticated")
    }
    if actor.Role != "admin" && actor.ID != r.ID {
        return errors.New("can only edit your own posts")
    }
    return nil
}
```

Wait — `Authorize` runs before decode, so `r.ID` is still zero when `Authorize` is called. That is a real limitation, and it is deliberate: see [04-authorization.md](04-authorization.md) for why, and what to do instead.

### Using the request

```go
func UpdatePost(c *gin.Context) {
    var req requests.UpdatePostRequest
    if err := reqval.BindGin(c, &req); err != nil {
        reqval.AbortWithError(c, err)
        return
    }
    // req.Title and req.Body are validated.
    // req.ID is whatever the client sent; you usually use the path parameter
    // from the route instead of the body field.
}
```

## Common patterns

### Request with only rules

```go
type SimpleRequest struct {
    reqval.Base
    Email string `json:"email"`
}

func (r *SimpleRequest) Rules() map[string][]string {
    return map[string][]string{"Email": {"required", "email"}}
}
```

### Request with rules and messages

```go
type SignupRequest struct {
    reqval.Base
    Email    string `json:"email"`
    Password string `json:"password"`
    PasswordConfirmation string `json:"password_confirmation"`
}

func (r *SignupRequest) Rules() map[string][]string {
    return map[string][]string{
        "Email":    {"required", "email"},
        "Password": {"required", "min:8", "confirmed"},
    }
}

func (r *SignupRequest) Messages() map[string]string {
    return map[string]string{
        "Email.email":      "That email does not look right.",
        "Password.min":     "Passwords must be at least {{param}} characters.",
        "Password.confirmed": "The two passwords do not match.",
    }
}
```

### Request with authorization

```go
type AdminOnlyRequest struct {
    reqval.Base
    Name string `json:"name"`
}

func (r *AdminOnlyRequest) Rules() map[string][]string {
    return map[string][]string{"Name": {"required"}}
}

func (r *AdminOnlyRequest) Authorize(ctx context.Context) error {
    actor, ok := ActorFrom(ctx)
    if !ok || actor.Role != "admin" {
        return errors.New("admin only")
    }
    return nil
}
```

### Request that accepts a file

```go
type AvatarRequest struct {
    reqval.Base
    Description string `form:"description"`
}

func (r *AvatarRequest) Rules() map[string][]string {
    return map[string][]string{
        "Description": {"required", "max:200"},
    }
}

func (r *AvatarRequest) FileRules() []reqval.FileRule {
    return []reqval.FileRule{
        {Field: "avatar", MaxBytes: 2 << 20, MIMEs: []string{"image/png", "image/jpeg"}},
    }
}
```

See [07-file-uploads.md](07-file-uploads.md) for the full story.

## Common mistakes

**Using JSON tag names as rule keys.** `"title": {"required"}` is wrong; `"Title": {"required"}` is right. The reflection walk uses Go field names, not tag names.

**Putting `validate:` tags on the struct.** That is `go-playground/validator` syntax, not `reqval` syntax. `reqval` reads rules from `Rules()`, not from tags. Tags on fields are for encoding/decoding only (`json:`, `form:`).

**Returning `nil` from `Rules()` when you meant "no rules."** `nil` and an empty map mean the same thing (no validation), but a typo like `return map[string][]string{}` (empty but non-nil) is also fine. There is no "required rules" enforcement — a request with no rules simply passes.

**Trying to use `r.ID` in `Authorize`.** The field is not populated yet. See [04-authorization.md](04-authorization.md).

## What to read next

- **[02-rules.md](02-rules.md)** — every rule, with input/output examples.
- **[03-messages.md](03-messages.md)** — per-request, per-app, and translated messages.
- **[04-authorization.md](04-authorization.md)** — the pre-decode ordering, and the `r.ID` caveat.

## Summary

A request file has four parts:

1. A struct embedding `reqval.Base`, with `json:` (or `form:`) tags.
2. `Rules()` — map of Go field name to rule strings.
3. `Messages()` — optional; map of `<Field>.<rule>` to template.
4. `Authorize(ctx)` — optional; `nil` to allow, error to deny.

Rules use Go field names, not JSON tag names. Order matters. Empty `Rules()` means no validation.
