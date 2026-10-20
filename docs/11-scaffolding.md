# Scaffolding

`reqval new request <Name>` is the `make:request` equivalent: a small CLI that writes a request file with `Rules()`, `Messages()`, and `Authorize()` stubbed out. This file explains how to install it, the flags, what it generates, and what it deliberately does not do.

## Install

Install the CLI binary alongside the library:

```bash
go install github.com/thomas666-beast/reqval/cmd/reqval@latest
```

The binary lands in `$GOBIN` (or `$GOPATH/bin`), which should be on your `PATH`. Verify:

```bash
reqval help
```

You should see:

```
reqval — Laravel-style form validation for Go

Usage:
  reqval new request <Name> [flags]

Flags:
  --package <pkg>   package name for the generated file (default: requests)
  --dir <path>      directory to write the file into   (default: .)
  --force           overwrite if the file exists

Example:
  reqval new request StorePostRequest
  reqval new request UpdatePostRequest --dir=internal/http/requests
```

If `reqval` is not found, `$GOBIN` is not on your `PATH`. Add it:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

## The first request

From the root of your project:

```bash
reqval new request StorePostRequest
```

Output:

```
created storepostrequest.go
```

The file `storepostrequest.go` in the current directory:

```go
package requests

import (
    "context"

    "github.com/thomas666-beast/reqval"
)

// StorePostRequest validates an incoming request.
type StorePostRequest struct {
    reqval.Base
}

func (r *StorePostRequest) Rules() map[string][]string {
    return map[string][]string{}
}

func (r *StorePostRequest) Messages() map[string]string {
    return map[string]string{}
}

func (r *StorePostRequest) Authorize(ctx context.Context) error {
    return nil
}
```

That is a compilable request. It passes any input because `Rules()` is empty. It compiles, runs, and gives you the scaffolding to fill in.

## What to do with the generated file

Three steps, in order.

### 1. Add the fields

```go
type StorePostRequest struct {
    reqval.Base

    Title      string `json:"title"`
    Body       string `json:"body"`
    CategoryID int64  `json:"category_id"`
}
```

The fields are the request body: the keys the client sends, the values the handler receives. The `json:` tag tells the decoder which JSON key to read.

### 2. Fill in `Rules()`

```go
func (r *StorePostRequest) Rules() map[string][]string {
    return map[string][]string{
        "Title":      {"required", "max:255"},
        "Body":       {"required", "min:10"},
        "CategoryID": {"required", "exists:categories,id"},
    }
}
```

### 3. Customize `Messages()` and `Authorize()` — or delete them

`Messages()` and `Authorize()` are stubs. If you do not need custom messages for this request, delete `Messages()`. If you do not need an authorization check, delete `Authorize()`. `reqval.Base` provides defaults for both, so the request still satisfies `reqval.Request` with only `Rules()` defined.

Deleting methods is fine. Leaving them empty is fine. Both work.

## Flags

### `--package`

The Go package name of the generated file. Default `requests`.

```bash
reqval new request StorePostRequest --package=http
```

Produces `package http` at the top of the file. Use this when your request files live alongside your handlers, not in a dedicated package.

### `--dir`

The directory to write into. Default `.` (current directory).

```bash
reqval new request StorePostRequest --dir=internal/http/requests
```

Produces `internal/http/requests/storepostrequest.go`. The directory is created if it does not exist.

Combining both:

```bash
reqval new request StorePostRequest --package=requests --dir=internal/http/requests
```

Produces a file in `internal/http/requests/storepostrequest.go`, starting with `package requests`.

### `--force`

Overwrites an existing file. Without it, the CLI refuses:

```
reqval: storepostrequest.go already exists (use --force to overwrite)
```

With `--force`:

```bash
reqval new request StorePostRequest --force
```

The existing file is replaced. **This is a complete overwrite**, not a merge. Any changes you had made — fields, rules, messages — are gone. Use `--force` only when you want to start over.

## File name convention

The generated file name is the lowercase form of the request name:

| Input name | File name |
| :--- | :--- |
| `StorePostRequest` | `storepostrequest.go` |
| `UpdatePostRequest` | `updatepostrequest.go` |
| `CreateUserRequest` | `createuserrequest.go` |
| `StorePostRequest` | `storepostrequest.go` |

There is no underscore or kebab-case transformation. `UserRegistrationRequest` becomes `userregistrationrequest.go`. That is the Go convention for file names: lowercase, no separators (or underscores if you prefer, but `reqval` uses the flat form).

If you prefer `user_registration_request.go`, rename the file after generation. The CLI does not have a flag for this.

## Name validation

The request name must be a valid exported Go identifier: starts with an uppercase letter, contains only letters, digits, and underscores. Anything else is rejected:

```bash
reqval new request storePostRequest
# reqval: "storePostRequest" is not a valid exported Go identifier

reqval new request 123Request
# reqval: "123Request" is not a valid exported Go identifier

reqval new request Store-Post-Request
# reqval: "Store-Post-Request" is not a valid exported Go identifier
```

The check is simple (`unicode.IsUpper` on the first rune, then alphanumeric and underscore), but it is enough to catch the common mistakes. If you need names that Go does not allow — you do not.

## What the CLI does not do

The generated file is a **skeleton**, not a working request. It does not:

- **Infer rules from a schema.** No database introspection, no OpenAPI parsing, no migration reading. You write the rules by hand.
- **Generate handler code.** Only the request file.
- **Generate tests.** You write the tests.
- **Register the request with a router.** Routing is your framework's job.
- **Migrate an existing request.** `--force` overwrites; there is no "add a field to an existing request" mode.

The CLI exists to save you the ten lines of boilerplate — package, imports, struct, three method stubs. It is not a code generator in the "from schema to app" sense.

## A realistic workflow

Starting a new endpoint:

```bash
# 1. Generate the request
reqval new request StorePostRequest --dir=internal/http/requests

# 2. Edit the request: fields, rules, messages, authorize
$EDITOR internal/http/requests/storepostrequest.go

# 3. Wire it into the handler
$EDITOR internal/http/handlers/post.go

# 4. Write a test
$EDITOR internal/http/requests/storepostrequest_test.go

# 5. Run
go test ./...
go run .
```

Step 1 is what the CLI covers. Steps 2 through 5 are yours. The CLI does not try to do more than step 1, because doing more would require assuming things about your project structure, your framework, and your testing style that the CLI cannot know.

## Compared to `php artisan make:request`

Laravel's `make:request` generates a file with `authorize()` and `rules()` already present, and stubs for `messages()`, `attributes()`, and `prepareForValidation()`. `reqval new request` generates `Rules()`, `Messages()`, and `Authorize()` — the same three that matter in the Go design.

The Laravel version also writes to `app/Http/Requests/` by convention. The `reqval` version writes to the current directory by default, or wherever `--dir` says. That is because Go projects do not have a single canonical location for HTTP-related code; your layout is your choice.

Laravel's `make:request` does not generate rules from a schema. Neither does `reqval new request`. The scaffold is a starting point, not a spec.

## Testing the scaffold itself

If you fork `reqval` or want to verify the CLI works after a change, the CLI has its own tests in `cmd/reqval/main_test.go`:

```go
func TestScaffoldCreatesFile(t *testing.T) {
    dir := t.TempDir()
    if err := scaffoldRequest("demo", dir, "StorePostRequest", false); err != nil {
        t.Fatalf("scaffold: %v", err)
    }
    path := filepath.Join(dir, "storepostrequest.go")
    b, err := os.ReadFile(path)
    if err != nil {
        t.Fatalf("read: %v", err)
    }
    s := string(b)
    for _, want := range []string{
        "package demo",
        "type StorePostRequest struct",
        "func (r *StorePostRequest) Rules() map[string][]string",
    } {
        if !strings.Contains(s, want) {
            t.Fatalf("missing %q in:\n%s", want, s)
        }
    }
}
```

Run with:

```bash
go test ./cmd/reqval
```

Three tests: creates a file, refuses to overwrite, rejects bad names. The coverage is what the CLI does, not what the generated code does.

## What to read next

- **[01-writing-a-request.md](01-writing-a-request.md)** — what to fill in after the scaffold.
- **[02-rules.md](02-rules.md)** — the rules you will put in `Rules()`.
- **[04-authorization.md](04-authorization.md)** — the `Authorize` method in depth.

## Summary

`go install github.com/thomas666-beast/reqval/cmd/reqval@latest` puts `reqval` on your path. `reqval new request Name` writes `<name>.go` with a compiling skeleton. Flags: `--package`, `--dir`, `--force`. The scaffold is a starting point — no rule inference, no handler generation, no test generation. Fill in fields, rules, and (optionally) messages and authorization. Delete the methods you do not need; `reqval.Base` provides defaults.
