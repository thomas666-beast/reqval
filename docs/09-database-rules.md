# Database rules

Two rules touch the database: `unique` and `exists`. This file explains how to wire a database, the four parameter forms of `unique`, the two of `exists`, the injection safety, and the caveat about concurrent writes.

## Setup

Register a database once at startup, before any request runs:

```go
package main

import (
    "database/sql"
    "github.com/thomas666-beast/reqval"
    _ "modernc.org/sqlite"
)

func main() {
    db, err := sql.Open("sqlite", "app.db")
    if err != nil {
        panic(err)
    }
    defer db.Close()

    reqval.RegisterDB(db)

    // ... start the server
}
```

`RegisterDB` accepts anything with a `QueryRowContext` method:

```go
type DBQuerier interface {
    QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
```

`*sql.DB`, `*sql.Tx`, `sqlx.DB`, and any wrapper that exposes this method all qualify. If you use an ORM (GORM, ent, etc.), wrap its connection in a small adapter:

```go
type GormAdapter struct {
    db *gorm.DB
}

func (a *GormAdapter) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
    return a.db.WithContext(ctx).Raw(query, args...).Row()
}
```

That is the entire integration surface. `reqval` never touches transactions, never begins or commits, and never inspects the driver. It runs one `SELECT COUNT(*)` per rule.

Registration is safe to call more than once; the most recent database wins. That is useful in tests, where each test creates its own in-memory database.

### PostgreSQL placeholder style

SQLite and MySQL use `?` as the placeholder. PostgreSQL uses `$1`, `$2`, etc.
`reqval` does not detect the driver — you declare the style at registration:

```go
db, _ := sql.Open("pgx", dsn)      // or "postgres"
reqval.RegisterDBWithPlaceholder(db, "$")
```

## `unique`

Checks that a value does not already exist in `<table>.<column>`. Fails if the count is greater than zero.

### Two parameters

```
"unique:users,email"
```

Generates:

```sql
SELECT COUNT(*) FROM users WHERE email = ?
```

Passes when zero rows match.

### Four parameters

```
"unique:users,email,id,42"
```

Generates:

```sql
SELECT COUNT(*) FROM users WHERE email = ? AND id <> ?
```

with `42` bound to the second parameter. This is the **update** case: you want to allow the row you are editing to keep its current email, but no other row may use it.

### The typical create + update pair

You usually have two request files, one for create and one for update:

```go
type StoreUserRequest struct {
    reqval.Base
    Username string `json:"username"`
    Email    string `json:"email"`
}

func (r *StoreUserRequest) Rules() map[string][]string {
    return map[string][]string{
        "Username": {"required", "unique:users,username"},
        "Email":    {"required", "email", "unique:users,email"},
    }
}
```

```go
type UpdateUserRequest struct {
    reqval.Base
    ID       int64  `json:"id"`
    Username string `json:"username"`
    Email    string `json:"email"`
}

func (r *UpdateUserRequest) Rules() map[string][]string {
    return map[string][]string{
        "Username": {"required", "unique:users,username,id," + strconv.FormatInt(r.ID, 10)},
        "Email":    {"required", "email", "unique:users,email,id," + strconv.FormatInt(r.ID, 10)},
    }
}
```

The update rules build the fourth parameter from `r.ID`, which is populated by the JSON decoder **before** validation runs. This is the payoff of method-based rules: the value is computed at runtime from the decoded struct.

### The `id` in the fourth parameter

The third parameter (`id`) is the **column name** the exclusion compares against. It is not the value. The fourth parameter is the value.

```
"unique:users,email,id,42"
         │     │     │  │
         │     │     │  └─ value: exclude rows where id = 42
         │     │     └──── column: compare against the id column
         │     └────────── column: the column being checked
         └──────────────── table
```

You can exclude by any column, not just `id`:

```go
"unique:posts,slug,author_id," + strconv.FormatInt(r.AuthorID, 10)
```

But `id` is the common case, and `id` is what the examples use.

### Skipping empty values

`unique` skips empty strings and zero values. An empty field never triggers the query. This is deliberate: `required` catches the empty case, and `unique` would otherwise do a pointless query on every empty submission.

```go
"Email": {"required", "unique:users,email"}
```

Empty email: `required` fails first (rule order), `unique` never runs. Non-empty email: `unique` runs.

If you want "empty is allowed and unique only applies when non-empty," omit `required`:

```go
"Email": {"email", "unique:users,email"}
```

Empty email: `email` fails (empty strings do not match the email regexp), so `unique` still never runs. If you want a truly optional email, you cannot use `email` or `unique` on it without more machinery — see "Optional fields" below.

## `exists`

Checks that a value **does** exist in `<table>.<column>`. Fails if the count is zero.

### Two parameters

```
"exists:categories,id"
```

Generates:

```sql
SELECT COUNT(*) FROM categories WHERE id = ?
```

Passes when at least one row matches.

### The typical foreign-key case

```go
type StorePostRequest struct {
    reqval.Base
    Title      string `json:"title"`
    CategoryID int64  `json:"category_id"`
}

func (r *StorePostRequest) Rules() map[string][]string {
    return map[string][]string{
        "Title":      {"required", "max:255"},
        "CategoryID": {"required", "exists:categories,id"},
    }
}
```

Empty `CategoryID` (`0`): `required` fails first, `exists` never runs. Non-zero `CategoryID`: `exists` runs; if there is no category with that ID, the field fails with the `exists` message.

The order matters. If you put `exists` before `required`:

```go
"CategoryID": {"exists:categories,id", "required"},
```

Then on `0`, `exists` runs first (skips `0` because it is empty), returns true, then `required` fails. Same result, different code path. `required` first is clearer.

### What `exists` does not check

`exists` only checks that the row exists. It does not check the row's state — whether the category is active, whether it belongs to the same tenant, etc. Those are additional rules you would implement yourself, or that you would validate in the handler after the existence check passes.

## SQL injection safety

Table names and column names cannot be parameterized in SQL. `reqval` therefore validates them against a regexp before splicing:

```go
identRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
```

Table and column names must match this. Anything containing `;`, quotes, spaces, dots, or other special characters is rejected. A rule with an invalid table or column fails closed (returns false), so the field shows a validation error.

Example: `unique:users;DROP TABLE users,email` is rejected. The injection attempt does not run.

The **values** are passed as `?` placeholders, which the Go driver parameterizes. No string concatenation of values.

The result: injection is impossible through either the rule params or the field values.

## Optional fields

There is no `nullable` or `sometimes` in `reqval` (yet). To make `unique` optional — meaning "if the field is present and non-empty, it must be unique; otherwise, skip" — you have two options.

### Option A: pointer fields

```go
type StoreUserRequest struct {
    reqval.Base
    Email *string `json:"email"`
}

func (r *StoreUserRequest) Rules() map[string][]string {
    return map[string][]string{
        "Email": {"email", "unique:users,email"},
    }
}
```

A `nil` pointer is "not sent." `isEmpty` sees it via `reflect.ValueOf(v).IsZero()` and returns true; `unique` skips. A non-nil pointer to `""` is `isEmpty` too. A non-nil pointer to `"a@b.com"` runs the rules.

The pointer is unwrapped in `checkRule` by `fv.Interface()`, so the underlying string is what `applyRule` sees. The rules behave as if the field were `string` when present, and skip when absent.

### Option B: separate request files

If optionality is the exception and requiredness is the norm, two files are simpler:

- `StoreUserRequest` requires email.
- `PatchUserRequest` has no email field at all.

The client chooses the endpoint. That is more work for the client but less machinery in the library.

`reqval` does not yet support `nullable`, `sometimes`, or `required_if`. These are on the roadmap and would be straightforward to add to the rule dispatcher. If you need them today, Option A or B is the workaround.

## Concurrency and the check-then-insert race

`unique` runs a `SELECT COUNT(*)`. It does not lock anything. Between the check and your `INSERT`, another request can insert a conflicting row. The count then went from 0 to 2, but your validation already returned success.

This is not a bug in `reqval`; it is inherent to check-then-insert. Laravel has the same issue. The mitigations are:

1. **Add a database unique index.** The index is the actual guarantee. `unique` is a UX improvement (better error messages), not a correctness guarantee. The index catches the race at insert time with a database error.
2. **Handle the database error.** After `BindGin` passes, your `INSERT` may still fail with a unique-constraint violation. Catch it and translate to a 422 with the same field error.

```go
if err := reqval.BindGin(c, &req); err != nil {
    reqval.AbortWithError(c, err)
    return
}
if _, err := db.Exec("INSERT INTO users (email) VALUES (?)", req.Email); err != nil {
    if isUniqueViolation(err) {
        c.JSON(422, gin.H{"errors": gin.H{"Email": "The Email has already been taken."}})
        return
    }
    c.JSON(500, gin.H{"error": "internal"})
    return
}
```

The `unique` rule is a first-pass filter; the database index is the actual guarantee. Use both.

## Testing

The library's own tests use an in-memory SQLite database:

```go
func newTestDB(t *testing.T) *sql.DB {
    t.Helper()
    db, err := sql.Open("sqlite", ":memory:")
    if err != nil {
        t.Fatalf("open sqlite: %v", err)
    }
    db.SetMaxOpenConns(1)
    t.Cleanup(func() { _ = db.Close() })
    // schema setup...
    return db
}
```

`SetMaxOpenConns(1)` is critical for `:memory:` SQLite. Each new connection to `:memory:` gets its own database; without the limit, a query on a second connection would see an empty schema. That is not a `reqval` issue, it is a SQLite `:memory:` issue — but it bites everyone who tests with in-memory SQLite, so it is worth knowing.

For your own tests:

```go
func TestUniqueCreate(t *testing.T) {
    reqval.RegisterDB(newTestDB(t))
    if err := reqval.ValidateCtx(context.Background(), &userReq{Username: "carol"}); err != nil {
        t.Fatalf("carol should be free: %v", err)
    }
    if _, ok := reqval.AsValidationError(reqval.ValidateCtx(context.Background(), &userReq{Username: "alice"})); !ok {
        t.Fatalf("alice should be taken")
    }
}
```

The `RegisterDB` call resets the database for the test. Because tests share the process, one test's database does not leak into another's as long as each test calls `RegisterDB` with its own.

## What to read next

- **[02-rules.md](02-rules.md)** — the full rule reference.
- **[08-nested-arrays.md](08-nested-arrays.md)** — database rules inside slices (with a performance warning).
- **[10-error-handling.md](10-error-handling.md)** — rendering the 422.

## Summary

Register a `DBQuerier` with `reqval.RegisterDB`. `unique:table,column` checks for non-existence. `unique:table,column,idCol,idValue` excludes the current row (update case). `exists:table,column` checks for existence. Empty values skip both. Table and column names are validated to prevent injection. `unique` is a first-pass filter, not a correctness guarantee — add a database unique index for that. `:memory:` SQLite needs `SetMaxOpenConns(1)`.
