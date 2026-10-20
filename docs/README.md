# reqval documentation

Start here. Each file in this directory covers one topic, with examples you can copy and run.

If you are new to `reqval`, read the files in order. If you are looking for something specific, jump to the file that matches.

## Getting started

| File | What it covers |
| :--- | :--- |
| [00-getting-started.md](00-getting-started.md) | Install, first request, first handler, first test |
| [01-writing-a-request.md](01-writing-a-request.md) | Anatomy of a request file: rules, messages, authorization |
| [02-rules.md](02-rules.md) | Every rule, with example inputs and expected outcomes |
| [03-messages.md](03-messages.md) | Per-request, per-app, and translated messages |
| [04-authorization.md](04-authorization.md) | Running `Authorize` before decode, and what 403 looks like |
| [05-binding-json.md](05-binding-json.md) | JSON bodies with `Bind`, `BindCtx`, `BindGin` |
| [06-binding-form.md](06-binding-form.md) | `application/x-www-form-urlencoded` with `BindForm*` |
| [07-file-uploads.md](07-file-uploads.md) | `multipart/form-data`, MIME, size limits |
| [08-nested-arrays.md](08-nested-arrays.md) | Paths like `Items.*.Name` |
| [09-database-rules.md](09-database-rules.md) | `unique`, `exists`, update exclusion |
| [10-error-handling.md](10-error-handling.md) | Error types, status codes, custom rendering |
| [11-scaffolding.md](11-scaffolding.md) | The `reqval new request` CLI |

## Conventions

Every example in these docs is runnable. Where an example needs a database, it uses an in-memory SQLite database so you can `go run` it without setup.

Examples use [Gin](https://github.com/gin-gonic/gin) for the HTTP layer because that is the most common choice. The framework-agnostic functions (`BindCtx`, `BindFormCtx`, `BindMultipartCtx`, `Validate`) are shown in [05](05-binding-json.md), [06](06-binding-form.md), and [10](10-error-handling.md) so you can use `reqval` with `net/http` or any other framework.

## Where to go next

If you want a single-file overview, read the project's main [README](../README.md). If you want working programs, look in [`examples/`](../examples/). If you want to see how the library itself is tested, read [`reqval_test.go`](../reqval_test.go).
