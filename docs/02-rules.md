# Rules

Every rule `reqval` understands, with a small input/output table for each. Rules are declared in `Rules()` as strings in the form `"name"` or `"name:param"`. Comma-separated params are a single string: `"unique:users,email,id,42"`.

The library does not hardcode English wording for any of these. Messages come from the request's `Messages()`, the application bundle, or a generic fallback. See [03-messages.md](03-messages.md).

## Summary

| Rule | Params | Applies to | Meaning |
| :--- | :--- | :--- | :--- |
| `required` | — | any | non-zero, non-empty |
| `min` | N | string / slice / number | length or value ≥ N |
| `max` | N | string / slice / number | length or value ≤ N |
| `email` | — | string | valid email |
| `url` | — | string | valid URL |
| `uuid` | — | string | valid UUID |
| `numeric` | — | string | only digits |
| `alpha` | — | string | only letters |
| `alphanum` | — | string | letters and digits |
| `gte` | N | number | value ≥ N |
| `lte` | N | number | value ≤ N |
| `in` | list | any | value is one of list |
| `oneof` | list | any | same as `in`, space-separated |
| `between` | lo,hi | number / string | numeric range or length range |
| `regexp` | pattern | string | matches Go regexp |
| `date` | layout? | string | parses as a date |
| `confirmed` | — | string | equals `<Field>Confirmation` |
| `unique` | table,col[,idCol,id] | string / number | not in DB |
| `exists` | table,col | string / number | in DB |

## `required`

Fails on zero values and on empty strings, slices, and maps.

```go
"Title":  {"required"}   // "" fails
"Age":    {"required"}   // 0 fails
"Tags":   {"required"}   // [] fails
```

Passes on `"x"`, `1`, `[]string{"a"}`, `true`.

Common pitfall: an int field that is genuinely allowed to be `0` should not use `required`. Use `gte:0` (which accepts `0`) if "present but may be zero" is the intent. There is no way for `reqval` to tell `0` (zero value, not sent) from `0` (sent) — the same limitation Laravel has without nullable fields.

## `min`, `max`

For strings, min/max compare length. For slices, arrays, and maps, they compare element count. For numbers, they compare value.

```go
"Name":  {"min:3"}     // length >= 3
"Name":  {"max:255"}   // length <= 255
"Tags":  {"min:1"}     // at least one tag
"Items": {"max:10"}    // at most ten items
"Age":   {"min:18"}    // value >= 18
```

## `email`, `url`, `uuid`

Format checks on strings.

```go
"Email":    {"email"}   // "a@b.com" passes, "nope" fails
"Website":  {"url"}     // "https://x" passes, "x" fails
"RequestID":{"uuid"}    // UUID v1-v5 shape
```

`email` uses a pragmatic regexp (`^[^@\s]+@[^@\s]+\.[^@\s]+$`). It rejects obvious garbage and accepts anything with `@` and a dot. It does not do full RFC 5322 validation — that would require parsing, and would reject addresses that some providers actually send.

## `numeric`, `alpha`, `alphanum`

Character-class checks on strings.

```go
"Code":     {"numeric"}   // "12345" passes, "12a" fails
"Username": {"alpha"}     // "alice" passes, "alice1" fails
"Username": {"alphanum"}  // "alice1" passes, "alice!" fails
```

## `gte`, `lte`

Numeric comparisons, for fields where you want "at least" / "at most" but not "must be present."

```go
"Age":   {"gte:18"}     // 18, 19, 100 pass; 17 fails
"Score": {"lte:120"}    // 0..120 pass; 121 fails
```

`gte:0` accepts zero. Combine with `required` if the field must also be present:

```go
"Age": {"required", "gte:18"}
```

## `in`, `oneof`

Membership test.

```go
"Role": {"in:admin,editor,viewer"}
"Role": {"in:admin editor viewer"}      // space form, same result
"Role": {"oneof:admin editor viewer"}   // validator-native spelling
```

`in` splits the param on commas, spaces, and colons. `oneof` splits on spaces only (it matches `go-playground/validator`'s syntax).

Values are compared as strings. `"in:1,2,3"` will match the string `"1"`, not the integer `1`. If your field is an int, use `between` instead:

```go
"Level": {"between:1,3"}   // 1, 2, 3 pass
```

## `between`

Numeric range, or length range for strings.

```go
"Age":    {"between:18,120"}    // 18..120 pass
"Score":  {"between:1,100"}     // 1..100 pass
"Name":   {"between:3,10"}      // length 3..10
```

Note the comma inside the params. It is a single string: `"between:18,120"`. Do not write `"between:18", "120"` — that is two rules, not one.

## `regexp`

Matches the value against a Go regular expression.

```go
"Code":  {"regexp:^[A-Z]{3}-[0-9]{3}$"}
"Phone": {"regexp:^\\+?[0-9]{7,15}$"}
```

If the pattern fails to compile, the rule fails (returns false) rather than panicking. Always test your regexp against a sample before deploying.

If the field is not a string, the rule fails — `regexp` only applies to strings.

## `date`

Parses the string against a Go time layout.

```go
"PublishedAt": {"date:2006-01-02"}            // ISO date
"CreatedAt":   {"date:2006-01-02T15:04:05Z"}  // RFC3339
"Timestamp":   {"date"}                       // defaults to 2006-01-02
```

The layout is exactly the second argument to `time.Parse`. If you are not familiar with Go layout strings, the important ones are:

| Layout | Matches |
| :--- | :--- |
| `2006-01-02` | `2024-03-15` |
| `2006-01-02 15:04:05` | `2024-03-15 14:30:00` |
| `2006-01-02T15:04:05Z` | `2024-03-15T14:30:00Z` |
| `RFC3339` | same as above (named layout) |

Empty strings pass; pair with `required` if the field must be present.

If you want to keep the string after validation and parse it to `time.Time` yourself:

```go
func (r *StoreRequest) Rules() map[string][]string {
    return map[string][]string{
        "PublishedAt": {"required", "date:2006-01-02"},
    }
}

// In the handler:
publishedAt, err := time.Parse("2006-01-02", req.PublishedAt)
if err != nil {
    // Should not happen — validation already passed — but handle defensively.
}
```

Alternatively, type the field as `time.Time` and let the JSON decoder handle it via a custom `UnmarshalJSON` — but then `date` is not the rule, `required` and a struct field are. That is outside `reqval`'s scope.

## `confirmed`

Cross-field: the field must equal a sibling field named `<Field>Confirmation` or `<field>_confirmation`.

```go
type SignupRequest struct {
    reqval.Base
    Password             string `json:"password"`
    PasswordConfirmation string `json:"password_confirmation"`
}

func (r *SignupRequest) Rules() map[string][]string {
    return map[string][]string{
        "Password": {"required", "min:8", "confirmed"},
    }
}
```

The confirmation field does **not** need a `validate` tag or a rule. It only needs to be exported and named one of the two forms.

`confirmed` in a nested path (`Items.*.Password`) compares against `<Field>Confirmation` on the same element.

## `unique`

Checks the database: the value must not already exist in `<table>.<column>`.

```
"unique:users,email"                 plain uniqueness
"unique:users,email,id,42"           exclude row where id = 42
```

Requires `reqval.RegisterDB(...)` before validation. See [09-database-rules.md](09-database-rules.md).

Empty strings and zero values skip the check. Use `required` first if the field must be present.

## `exists`

Checks the database: the value must exist in `<table>.<column>`.

```
"exists:categories,id"
```

Requires `reqval.RegisterDB(...)`. Zero values skip the check; `required` catches them.

## Combining rules

Rules are comma-separated in the slice, colon after the name, and run left-to-right. First failure wins.

```go
"Email":    {"required", "email"}
"Password": {"required", "min:8", "confirmed"}
"Website":  {"omitempty"}       // nothing to skip without omitempty support, but the list is empty anyway
```

`reqval` does not (currently) support `omitempty`. To skip validation on empty fields, omit the rule — the field is empty, so there is nothing to validate against. The one exception is `unique` and `exists`, which naturally skip empties.

If you need "this rule only runs when the field is non-empty," write a rule that returns true for empty (like `email` does — empty strings do not match the email regexp, so `email` would fail on empty). Use `required` to require presence, or omit the rule entirely.

## Nested paths

Rule keys may be paths. See [08-nested-arrays.md](08-nested-arrays.md).

```go
"Items.*.Name":     {"required", "max:50"}
"Items.*.Quantity": {"required", "gte:1"}
```

## What to read next

- **[03-messages.md](03-messages.md)** — how to change the messages these rules produce.
- **[08-nested-arrays.md](08-nested-arrays.md)** — the `Items.*.Name` syntax.
- **[09-database-rules.md](09-database-rules.md)** — `unique` and `exists` in depth.

## Summary

Each rule is a string in `Rules()`. Names are fixed. Params follow a colon. Multiple params are comma-separated inside one string. Rules run in order, stop on first failure. `min`/`max` on slices count elements. `date` uses Go layouts. `confirmed` compares a sibling. `unique`/`exists` need a registered database.
