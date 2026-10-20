package reqval

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

// BindFormCtx parses an application/x-www-form-urlencoded body into dst,
// runs Authorize, then Rules(). Mirrors BindCtx but for form bodies.
func BindFormCtx(ctx context.Context, r *http.Request, dst any) error {
	if err := runAuthorizer(ctx, dst); err != nil {
		return err
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/x-www-form-urlencoded"
	}
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("reqval: invalid content-type: %w", err)
	}
	if mt != "application/x-www-form-urlencoded" {
		return fmt.Errorf("reqval: expected application/x-www-form-urlencoded, got %s", mt)
	}

	if err := r.ParseForm(); err != nil {
		return err
	}

	if err := decodeForm(r.PostForm, dst); err != nil {
		return err
	}

	return ValidateCtx(ctx, dst)
}

// BindForm reads a form body from r, decodes into dst, and validates.
//
// Prefer BindFormCtx when you have an *http.Request; this function exists
// for callers that already hold the body (e.g., in tests).
func BindForm(r io.Reader, contentType string, dst any) error {
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, "/", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return BindFormCtx(context.Background(), req, dst)
}
