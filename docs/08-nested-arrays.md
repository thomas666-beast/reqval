# Nested arrays

`reqval` supports path keys in `Rules()`. A dot separates segments. `*` iterates a slice, array, or map. This file explains the syntax, the error-key shape, and the cases that do and do not work.

## The problem this solves

A struct can hold a slice of structs:

```go
type Item struct {
    Name     string `json:"name"`
    Quantity int    `json:"quantity"`
}

type OrderRequest struct {
    reqval.Base
    Items []Item `json:"items"`
}
```

The client sends:

```json
{
    "items": [
        {"name": "widget", "quantity": 2},
        {"name": "gadget", "quantity": 1}
    ]
}
```

You want every item to have a name and a positive quantity. With plain field keys you cannot express that — `"Items"` is one field, and there is no `"Name"` field on the top-level struct. `reqval` solves this with path keys.

## The syntax

Rule keys may be dot-separated paths. Each segment is one of:

| Segment | Meaning |
| :--- | :--- |
| `Field` | a field on the current struct |
| `0`, `1`, `42` | a specific index of a slice or array |
| `key` | a specific key of a map (string key) |
| `*` | every element of a slice, array, or map |

Example rules:

```go
func (r *OrderRequest) Rules() map[string][]string {
    return map[string][]string{
        "Items.*.Name":     {"required", "max:50"},
        "Items.*.Quantity": {"required", "gte:1"},
    }
}
```

- `Items.*.Name` means: for each element of `Items`, validate the `Name` field.
- `Items.*.Quantity` means: for each element, validate the `Quantity` field.

Every element is checked independently. The rule is not "at least one item has a name" — it is "every item has a name."

## The error key

If an element fails, the error key in `ValidationError.Fields` includes the index:

```json
{
    "errors": {
        "Items.1.Name": "The Name field is invalid.",
        "Items.0.Quantity": "The Quantity field is invalid."
    }
}
```

The client sees which element failed, not just that "something about Items failed."

If multiple elements fail the same rule, each produces its own key:

```json
{
    "errors": {
        "Items.0.Name": "The Name field is invalid.",
        "Items.2.Name": "The Name field is invalid."
    }
}
```

Each element is checked independently; the first failure on an element wins for that element, but other elements are still checked.

## A complete example

```go
package requests

import (
    "context"
    "github.com/thomas666-beast/reqval"
)

type Item struct {
    Name     string `json:"name"`
    Quantity int    `json:"quantity"`
}

type OrderRequest struct {
    reqval.Base
    CustomerEmail string `json:"customer_email"`
    Items         []Item `json:"items"`
}

func (r *OrderRequest) Rules() map[string][]string {
    return map[string][]string{
        "CustomerEmail":    {"required", "email"},
        "Items.*.Name":     {"required", "max:50"},
        "Items.*.Quantity": {"required", "gte:1"},
    }
}

func (r *OrderRequest) Messages() map[string]string {
    return map[string]string{
        "Name.required":     "Every item needs a name.",
        "Quantity.gte":      "Quantity must be at least {{param}}.",
    }
}

func (r *OrderRequest) Authorize(ctx context.Context) error {
    return nil
}
```

Note the message keys: `"Name.required"`, not `"Items.*.Name.required"`. Message lookup uses the **bare field name** — the last segment of the path. A single message covers every element.

The error key, by contrast, is the full path: `Items.0.Name`, `Items.2.Name`, etc. The distinction matters:

- **Message lookup** uses `Name` (bare), so one template applies to all items.
- **Error key** uses `Items.1.Name` (full), so the client can point at the specific element.

## Nested slices

Paths go arbitrarily deep:

```go
type LineItem struct {
    SKU string `json:"sku"`
}

type Order struct {
    reqval.Base
    Batches []struct {
        Items []LineItem `json:"items"`
    } `json:"batches"`
}

func (r *Order) Rules() map[string][]string {
    return map[string][]string{
        "Batches.*.Items.*.SKU": {"required"},
    }
}
```

Each `*` iterates the slice at that level. A failure on the third item of the second batch produces `Batches.1.Items.2.SKU`.

## Specific indices

Use a number to target one element:

```go
"Items.0.Name": {"required"}   // the first item must have a name
```

That is rarely useful in practice — usually you validate every element or none. But it works, and it composes:

```go
"Items.0.Name": {"required"},
"Items.*.Name": {"required", "max:50"},
```

Both rules apply to `Items.0.Name`; the second one covers elements 1 and beyond. The first failure wins for that element.

## Maps

`*` iterates map entries. The map key (stringified) goes into the path:

```go
type Config struct {
    reqval.Base
    Settings map[string]struct {
        Value string `json:"value"`
    } `json:"settings"`
}

func (r *Config) Rules() map[string][]string {
    return map[string][]string{
        "Settings.*.Value": {"required"},
    }
}
```

A failure on `Settings["theme"].Value` produces the key `Settings.theme.Value`.

Map iteration order is not deterministic — Go randomizes it. If two entries fail, the order of the errors in the JSON object varies. That is fine for a map of errors, but do not write tests that assume a specific order.

## `confirmed` inside a path

`confirmed` works on nested fields. `Items.*.Password` compares each element's `Password` against that same element's `PasswordConfirmation`:

```go
type Account struct {
    Password             string `json:"password"`
    PasswordConfirmation string `json:"password_confirmation"`
}

type BulkSignup struct {
    reqval.Base
    Accounts []Account `json:"accounts"`
}

func (r *BulkSignup) Rules() map[string][]string {
    return map[string][]string{
        "Accounts.*.Password": {"required", "min:8", "confirmed"},
    }
}
```

If `Accounts[2].Password` does not match `Accounts[2].PasswordConfirmation`, the error key is `Accounts.2.Password` and the message is the configured `Password.confirmed` message.

That is the correct semantics: each account's password confirms against that account's own confirmation, not against another element's.

## Database rules inside a path

`unique` and `exists` work on nested fields too:

```go
type BulkCreate struct {
    reqval.Base
    Users []struct {
        Email string `json:"email"`
    } `json:"users"`
}

func (r *BulkCreate) Rules() map[string][]string {
    return map[string][]string{
        "Users.*.Email": {"required", "email", "unique:users,email"},
    }
}
```

Each email triggers its own database query. For N users, that is N queries. If N is large, this is expensive — see the note on performance below.

## Performance

Every rule on every element runs independently. `Items.*.Quantity` with `{"required", "gte:1"}` and 100 items means 200 rule evaluations. That is fine for normal forms. It is not fine for a request that carries 100,000 items.

For large payloads:

- Validate only the top-level shape (`"Items": {"required", "min:1", "max:100"}`) and do per-element checks in the handler.
- Or batch the queries for `unique`/`exists` into a single `IN (...)` and check the result in the handler.
- Or paginate the client — a 100,000-item request is a design problem, not a validation problem.

`reqval` does not have a built-in "batch" mode. The path syntax is for ergonomics on small-to-medium slices.

## Pointers in paths

A path segment on a pointer field dereferences automatically:

```go
type Wrapper struct {
    Inner *Inner `json:"inner"`
}

type Inner struct {
    Value string `json:"value"`
}

"Wrapper.Inner.Value": {"required"}
```

If the pointer is `nil`, the path resolves to nothing, and the rule produces an error on that path. That is the right behavior: "you said the field is required, but the parent object was null."

If you want to allow `nil` and skip the rules, use a slice (empty slice → zero iterations → no errors) or validate in the handler.

## Nil slices

`Items` is `nil` (the client sent `{"items": null}` or omitted the field). The path `Items.*.Name` iterates zero elements — no errors on `Name`. That means an empty list always passes per-element checks:

```json
{"items": []}
```

passes `Items.*.Name: required`. To require at least one item, add:

```go
"Items": {"required", "min:1"},
```

`required` on a slice means "non-empty." `min:1` on a slice means "at least one element." Combine both for "at least one item, every item valid."

## Testing nested rules

The library's own test file has three:

```go
func TestNestedValid(t *testing.T) {
    r := &orderReq{Items: []Item{
        {Name: "widget", Quantity: 2},
        {Name: "gadget", Quantity: 1},
    }}
    if err := reqval.ValidateCtx(context.Background(), r); err != nil {
        t.Fatalf("want valid, got %v", err)
    }
}

func TestNestedOneFails(t *testing.T) {
    r := &orderReq{Items: []Item{
        {Name: "widget", Quantity: 2},
        {Name: "", Quantity: 1},
    }}
    err := reqval.ValidateCtx(context.Background(), r)
    ve, ok := reqval.AsValidationError(err)
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
    err := reqval.ValidateCtx(context.Background(), r)
    ve, _ := reqval.AsValidationError(err)
    for _, want := range []string{"Items.0.Name", "Items.0.Quantity", "Items.1.Quantity"} {
        if _, has := ve.Fields[want]; !has {
            t.Fatalf("want %s flagged, got %v", want, ve.Fields)
        }
    }
}
```

The pattern: construct a request, call `ValidateCtx`, assert on `ve.Fields[<path>]`.

## What to read next

- **[02-rules.md](02-rules.md)** — the rules you can use on nested fields.
- **[03-messages.md](03-messages.md)** — the message-key vs error-key distinction, in more depth.
- **[09-database-rules.md](09-database-rules.md)** — `unique` and `exists` in nested paths.

## Summary

Rule keys may be paths: `Items.*.Name`, `Settings.*.Value`, `Batches.*.Items.*.SKU`. `*` iterates slices, arrays, and maps. Errors carry the full path (`Items.1.Name`); messages use the bare field name (`Name`). `confirmed` and database rules work inside paths. Nil slices produce no per-element errors; add `"Items": {"required", "min:1"}` to require at least one. Every element is validated independently — the cost is linear in element count.
