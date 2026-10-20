# Messages

`reqval` does not hardcode English strings for its rules. The library emits exactly one string on its own: `"The <field> field is invalid."`, used as a last resort when nothing else applies. Every other message comes from you: per-request, per-application, or per-locale.

This file explains the three sources, the priority order, and how to translate.

## The three sources

When a rule fails, `reqval` looks for a message in three places, in this order:

1. **The request's `Messages()` method** — highest priority.
2. **The application default bundle** — set once at startup with `reqval.SetDefaultMessages`.
3. **The library fallback** — `"The <field> field is invalid."`

The first source that has a matching key wins. If none do, the fallback applies.

## Per-request messages

`Messages()` returns `map[string]string` keyed by `<Field>.<rule>`:

```go
func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "Title.required": "We really need a title.",
        "Body.min":       "The body must be at least {{param}} characters.",
    }
}
```

Key format: `<Field>.<rule>` — the Go field name, a dot, then the rule name.

Placeholders:

- `{{field}}` — the bare field name, e.g. `Title`.
- `{{param}}` — the rule's parameter, e.g. `10` for `min:10`, or `admin,editor` for `in:admin,editor`.

Example with both placeholders:

```go
"Title.max": "{{field}} may not exceed {{param}} characters."
```

A failed `max:255` on `Title` produces:

```
Title may not exceed 255 characters.
```

### The wildcard form

`*.<rule>` applies one template to every field that fails that rule:

```go
func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "*.required": "Please fill in {{field}}.",
    }
}
```

Now any `required` failure in this request reads `Please fill in <field>.` — including nested paths (`Items.0.Name` will read `Please fill in Name.`, because `{{field}}` is the bare field name).

A field-specific key wins over a wildcard:

```go
"*.required":     "Please fill in {{field}}.",
"Title.required": "We really need a title.",
```

`Title` failures use the specific message; `Body` failures use the wildcard.

## Application default bundle

Set once at startup:

```go
func main() {
    reqval.SetDefaultMessages(reqval.ZH())
    // ... rest of the app
}
```

Or with a custom bundle:

```go
reqval.SetDefaultMessages(map[string]string{
    "*.required": "This field is required.",
    "*.email":    "This is not a valid email.",
})
```

The bundle is consulted for every request that has no matching key in its own `Messages()`. It is process-global; the last call wins, so set it in `main()` before starting the server.

To clear the bundle (useful in tests):

```go
reqval.SetDefaultMessages(nil)
```

### A custom bundle for another language

```go
func FrenchBundle() map[string]string {
    return map[string]string{
        "*.required":  "Le champ {{field}} est obligatoire.",
        "*.min":       "Le champ {{field}} doit contenir au moins {{param}} caractères.",
        "*.max":       "Le champ {{field}} ne peut pas dépasser {{param}} caractères.",
        "*.email":     "Le champ {{field}} doit être une adresse email valide.",
        "*.unique":    "La valeur {{field}} est déjà utilisée.",
    }
}

func main() {
    reqval.SetDefaultMessages(FrenchBundle())
}
```

There is no registry of languages. A bundle is just a `map[string]string`. You can have as many as you like and pick one at startup:

```go
switch os.Getenv("LANG") {
case "fr":
    reqval.SetDefaultMessages(FrenchBundle())
case "zh":
    reqval.SetDefaultMessages(reqval.ZH())
default:
    reqval.SetDefaultMessages(EnglishBundle())
}
```

## The Chinese bundle

`reqval.ZH()` returns a ready-made Chinese bundle for every rule:

```go
reqval.SetDefaultMessages(reqval.ZH())
```

It is a plain function returning a `map[string]string`, so you can inspect it, extend it, or override parts of it:

```go
bundle := reqval.ZH()
bundle["*.required"] = "{{field}} 不能为空。"   // your own wording
reqval.SetDefaultMessages(bundle)
```

## Combining all three

A realistic setup:

```go
// main.go
func main() {
    reqval.SetDefaultMessages(defaultBundle())
    // ...
}

func defaultBundle() map[string]string {
    return map[string]string{
        "*.required": "This field is required.",
        "*.email":    "Not a valid email address.",
        "*.min":      "Must be at least {{param}} characters.",
        "*.max":      "Must be at most {{param}} characters.",
    }
}
```

Then a request that wants a specific message overrides just that one:

```go
func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{
        "Title.required": "Every post needs a title.",
    }
}
```

Result:

| Failure | Message | Source |
| :--- | :--- | :--- |
| `Title.required` | `Every post needs a title.` | request |
| `Body.required` | `This field is required.` | app bundle |
| `Body.max` | `Must be at most 255 characters.` | app bundle |
| `CategoryID.exists` (no bundle entry) | `The CategoryID field is invalid.` | fallback |

## Messages for nested paths

When a rule fails on `Items.1.Name`, the field name passed to `messageFor` is the bare `Name`, not `Items.1.Name`. So a message like `"Items.*.Name.required"` does not work. Use the bare field name:

```go
func (r *OrderRequest) Messages() map[string]string {
    return map[string]string{
        "Name.required": "Each item needs a name.",
    }
}
```

Both `Items.0.Name` and `Items.1.Name` failures use that message.

The error key in `ValidationError.Fields` is still `Items.1.Name` — the full path. Only the message lookup uses the bare name. That means a client seeing the 422 can render `Item 2: Each item needs a name.` by combining the path and the message.

## What messages cannot do

- **Change the error key.** The key in `ValidationError.Fields` is always the field name or path, never overridden.
- **Reference sibling fields.** There is no `{{otherField}}` placeholder. If you want "Passwords do not match" to include the other field's value (which you should not — that is a security leak), do it in the handler.
- **Be functions.** Messages are strings, not closures. They cannot depend on other fields or computed state at message-build time. If you need that, build the request with a `Messages()` method that returns different strings based on other fields — but be aware that `Messages()` is called once per failure, so it sees the current state of the request.
- **Fail safe for translation.** Every key in a bundle is optional. If you set only `*.required` and forget `*.email`, the `email` rule produces the generic fallback. That is by design: an incomplete bundle still produces messages; it just produces less helpful ones for rules you have not translated.

## Testing message resolution

The cleanest test is to construct a request, force a failure, and check the message:

```go
func TestCustomMessage(t *testing.T) {
    req := &StorePostRequest{Body: "long enough text"}
    err := reqval.Validate(req)
    ve, ok := reqval.AsValidationError(err)
    if !ok {
        t.Fatalf("want validation error, got %v", err)
    }
    if ve.Fields["Title"] != "We really need a title." {
        t.Fatalf("want custom message, got %q", ve.Fields["Title"])
    }
}
```

The library's own test file (`reqval_test.go`) does exactly this for the three levels.

## What to read next

- **[04-authorization.md](04-authorization.md)** — messages on `AuthorizationError` are the `Message` field, not the messages bundle.
- **[10-error-handling.md](10-error-handling.md)** — how to render `ValidationError.Fields` as JSON, flash, or HTML.

## Summary

Three sources, resolved in order: request's `Messages()`, application bundle (`SetDefaultMessages`), generic fallback. Keys are `<Field>.<rule>` or `*.<rule>`. Placeholders: `{{field}}`, `{{param}}`. Nested-path messages use the bare field name. `reqval.ZH()` is a ready-made Chinese bundle; write your own the same way — it is just a map.
