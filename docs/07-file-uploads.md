# File uploads

`reqval` handles `multipart/form-data` with file constraints. This file explains the `FileRule` type, how MIME detection works, and how to wire it into a handler.

## Two interfaces, not one

A request that accepts files implements `reqval.Request` **and** `reqval.FileRequest`:

```go
type Request interface {
    Rules() map[string][]string
    Messages() map[string]string
    Authorize(ctx context.Context) error
}

type FileRequest interface {
    FileRules() []FileRule
}
```

`FileRules()` returns a slice of rules, one per file field. A request that has no files does not need to implement `FileRequest` — the multipart binder checks for it and skips file validation if the method is absent.

## The `FileRule` type

```go
type FileRule struct {
    Field    string   // form field name, e.g. "avatar"
    MaxBytes int64    // 0 = no upper limit
    MinBytes int64    // 0 = no lower limit
    MIMEs    []string // allowed MIME types; empty = any
}
```

| Field | Meaning | Example |
| :--- | :--- | :--- |
| `Field` | The `name` attribute of the file input | `"avatar"` |
| `MaxBytes` | Maximum size in bytes | `2 << 20` (2 MiB) |
| `MinBytes` | Minimum size in bytes | `1024` (1 KiB) |
| `MIMEs` | Allowed MIME types | `[]string{"image/png", "image/jpeg"}` |

If `MaxBytes` or `MinBytes` is `0`, that bound is not checked. If `MIMEs` is empty, any MIME type passes.

## A complete request

```go
package requests

import (
    "context"
    "github.com/thomas666-beast/reqval"
)

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
        {
            Field:    "avatar",
            MaxBytes: 2 << 20, // 2 MiB
            MIMEs:    []string{"image/png", "image/jpeg"},
        },
    }
}

func (r *AvatarRequest) Messages() map[string]string {
    return map[string]string{
        "avatar.mimes": "Avatar must be a PNG or JPEG.",
    }
}

func (r *AvatarRequest) Authorize(ctx context.Context) error {
    return nil
}
```

The request has two concerns:

- `Rules()` for the text fields in the form (here, `Description`).
- `FileRules()` for the file fields.

The `Description` field uses `form:"description"` because it arrives in the multipart body alongside the file. The file itself is accessed via the multipart form, not via a struct field — you never store the file in the request struct. That is deliberate: `reqval` validates files but leaves the `*multipart.FileHeader` to the handler, so you decide where the bytes go (local disk, S3, in-memory).

## A complete Gin handler

```go
func UploadAvatar(c *gin.Context) {
    var req requests.AvatarRequest
    if err := reqval.BindMultipartGin(c, &req); err != nil {
        reqval.AbortWithError(c, err)
        return
    }

    // The form fields are validated:
    log.Println("description:", req.Description)

    // The file is not in the struct. Access it from the parsed form:
    headers := c.Request.MultipartForm.File["avatar"]
    if len(headers) > 0 {
        f, err := headers[0].Open()
        if err != nil {
            c.AbortWithStatusJSON(500, gin.H{"error": "cannot open file"})
            return
        }
        defer f.Close()
        // f is an io.Reader; copy it to disk, S3, etc.
    }

    c.JSON(201, gin.H{"description": req.Description})
}
```

The flow:

1. `BindMultipartGin` parses the multipart body, decodes text fields into `req`, validates text fields with `Rules()`, and validates files with `FileRules()`.
2. On success, `c.Request.MultipartForm` is populated with the parsed form and files.
3. The handler opens `MultipartForm.File["avatar"]` and copies the bytes wherever they belong.

## A complete `net/http` handler

```go
func UploadAvatar(w http.ResponseWriter, r *http.Request) {
    var req requests.AvatarRequest
    if err := reqval.BindMultipartCtx(r.Context(), r, &req); err != nil {
        if ve, ok := reqval.AsValidationError(err); ok {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(422)
            json.NewEncoder(w).Encode(map[string]any{"errors": ve.Fields})
            return
        }
        http.Error(w, err.Error(), 400)
        return
    }
    headers := r.MultipartForm.File["avatar"]
    // ... open, copy, save
    json.NewEncoder(w).Encode(map[string]string{"description": req.Description})
}
```

Same flow, different plumbing.

## How MIME detection works

`reqval` does **not** trust the client's `Content-Type` header on the file part. That header is supplied by the browser (or by an attacker) and is trivially forged.

Instead, `reqval` reads the first 512 bytes of the file and calls `http.DetectContentType`, which detects the format from the magic bytes. That is how a `.png` file whose contents are actually plain text gets rejected: the magic bytes say "plain text," the allowed list says `image/png`, and the rule fails.

The detected MIME is stripped of the `; charset=...` suffix before comparison, so `text/plain; charset=utf-8` becomes `text/plain`. You can therefore list the bare type in `MIMEs`:

```go
MIMEs: []string{"text/plain"}   // matches "text/plain" and "text/plain; charset=utf-8"
```

Common types:

| Content | Detected MIME |
| :--- | :--- |
| PNG image | `image/png` |
| JPEG image | `image/jpeg` |
| GIF image | `image/gif` |
| PDF | `application/pdf` |
| Plain text | `text/plain; charset=utf-8` (stripped to `text/plain`) |
| HTML | `text/html; charset=utf-8` (stripped to `text/html`) |
| ZIP archive | `application/zip` |
| Unknown binary | `application/octet-stream` |

For a full list, see the `net/http` source or run `DetectContentType` on a sample.

### What MIME detection does not do

It does not scan the entire file. A file that starts with PNG magic bytes and continues with garbage will pass MIME detection. It also does not check for viruses, resize images, or verify that a PDF is actually a valid PDF.

If you need those checks, do them in the handler after `BindMultipartGin` returns successfully. `reqval`'s job ends at "this file's header says it is a PNG and it is under 2 MiB." The rest is application logic.

## Size checks

`MaxBytes` and `MinBytes` are byte counts. The file header's `Size` field is populated by the multipart parser; `reqval` compares against it without reading the file.

```go
{Field: "avatar", MaxBytes: 2 << 20}  // 2 MiB
{Field: "avatar", MinBytes: 1024}     // at least 1 KiB
{Field: "avatar", MaxBytes: 10 << 20, MinBytes: 1024}
```

`1 << 20` is 1 MiB (`1,048,576` bytes). `2 << 20` is 2 MiB. The shift form is common because it is easier to read than `2097152`.

### The multipart memory limit

`BindMultipartCtx` calls `r.ParseMultipartForm(32 << 20)`. That tells the Go multipart parser to keep up to 32 MiB in memory and spill the rest to temporary files. Files above 32 MiB are streamed to `/tmp` (or the OS temp directory), not held in memory.

If your `MaxBytes` is bigger than 32 MiB, raise the argument. For example, for 100 MiB uploads:

```go
// This is inside BindMultipartCtx; you cannot change it per-request.
// If you need larger, pre-parse the form yourself in the handler,
// then call reqval.Validate instead of BindMultipartCtx.
```

`reqval` does not currently expose the limit as a parameter. If you need more than 32 MiB, the pattern is:

```go
if err := r.ParseMultipartForm(100 << 20); err != nil {
    // handle
}
var req requests.AvatarRequest
// decode text fields manually, then:
reqval.ValidateCtx(r.Context(), &req)
```

Or open a PR to make the limit configurable. The current `32 << 20` is a reasonable default for most web forms.

## The 422 response

A failure on a file rule produces a `*ValidationError` whose `Fields` map contains the file's field name as the key:

```json
{"errors":{"avatar":"Avatar must be a PNG or JPEG."}}
```

The message comes from the same message resolution as any other rule. `avatar.mimes` in `Messages()` supplies it. If you do not override it, the fallback applies:

```json
{"errors":{"avatar":"The avatar field is invalid."}}
```

`mimes` is the rule name used internally. If a file fails both the MIME check and the size check (unlikely but possible), the MIME check runs first — see `checkOneFile` for the order.

## Multiple files, multiple rules

`FileRules()` returns a slice, so you can declare one rule per file field:

```go
func (r *ProductRequest) FileRules() []reqval.FileRule {
    return []reqval.FileRule{
        {Field: "cover",  MaxBytes: 5 << 20, MIMEs: []string{"image/png", "image/jpeg"}},
        {Field: "gallery", MaxBytes: 2 << 20, MIMEs: []string{"image/png"}},
        {Field: "manual",  MaxBytes: 20 << 20, MIMEs: []string{"application/pdf"}},
    }
}
```

Multiple files under the same field (e.g. `gallery[]` from the client) are all checked against the same rule. The first failure produces the field error.

## Missing file

If a field in `FileRules()` has no corresponding file in the multipart form, the rule produces a `required` failure:

```json
{"errors":{"avatar":"The avatar field is invalid."}}
```

There is no separate `optional` flag. If a file is optional, do not include it in `FileRules()` — instead, check in the handler:

```go
if len(c.Request.MultipartForm.File["avatar"]) > 0 {
    // file was sent; validate it (or trust the handler)
}
```

Or split the request: one that requires the file, one that does not.

## Testing file uploads

Build the multipart body with `mime/multipart`:

```go
func buildMultipart(t *testing.T, desc, filename string, content []byte) (*bytes.Buffer, string) {
    t.Helper()
    var buf bytes.Buffer
    w := multipart.NewWriter(&buf)
    if desc != "" {
        _ = w.WriteField("description", desc)
    }
    if filename != "" {
        fw, err := w.CreateFormFile("avatar", filename)
        if err != nil {
            t.Fatal(err)
        }
        fw.Write(content)
    }
    if err := w.Close(); err != nil {
        t.Fatal(err)
    }
    return &buf, w.FormDataContentType()
}
```

Then use `httptest`:

```go
func TestUploadValidPNG(t *testing.T) {
    body, ctype := buildMultipart(t, "my avatar", "avatar.png", []byte{
        0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, // PNG magic
        0, 0, 0, 0, 0, 0, 0, 0,
    })
    req := httptest.NewRequest(http.MethodPost, "/upload", body)
    req.Header.Set("Content-Type", ctype)

    var dst AvatarRequest
    if err := reqval.BindMultipartCtx(context.Background(), req, &dst); err != nil {
        t.Fatalf("want valid upload, got %v", err)
    }
}
```

The `0x89, 'P', 'N', 'G'` prefix is what `DetectContentType` looks for. It does not need to be a real PNG — just the magic bytes.

For a wrong-MIME test:

```go
body, ctype := buildMultipart(t, "my avatar", "avatar.txt", []byte("plain text"))
req := httptest.NewRequest(http.MethodPost, "/upload", body)
req.Header.Set("Content-Type", ctype)
var dst AvatarRequest
err := reqval.BindMultipartCtx(context.Background(), req, &dst)
ve, ok := reqval.AsValidationError(err)
if !ok {
    t.Fatalf("want validation error, got %v", err)
}
if _, has := ve.Fields["avatar"]; !has {
    t.Fatalf("want avatar flagged, got %v", ve.Fields)
}
```

## What to read next

- **[06-binding-form.md](06-binding-form.md)** — the text-only form path.
- **[10-error-handling.md](10-error-handling.md)** — rendering the 422.

## Summary

Implement `FileRequest` with `FileRules()` to declare file constraints. Use `BindMultipartGin` or `BindMultipartCtx` for multipart bodies. MIME is detected from magic bytes, not the client header. `MaxBytes`/`MinBytes` compare against the file header's size. Failures appear in `ValidationError.Fields` under the file's field name. The handler opens the file from `MultipartForm.File[field]` and copies it wherever it belongs.
