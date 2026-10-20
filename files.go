package reqval

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

// FileRule describes constraints on an uploaded file.
type FileRule struct {
	Field    string   // form field name (e.g. "avatar")
	MaxBytes int64    // 0 = no upper limit
	MinBytes int64    // 0 = no lower limit
	MIMEs    []string // allowed MIME types; empty = any
}

// FileRequest is optionally implemented by requests that accept uploads.
type FileRequest interface {
	FileRules() []FileRule
}

func validateFiles(req any, form *multipart.Form) map[string]string {
	fr, ok := req.(FileRequest)
	if !ok {
		return nil
	}
	rules := fr.FileRules()
	if len(rules) == 0 {
		return nil
	}

	r, _ := req.(Request)
	fields := map[string]string{}

	for _, rule := range rules {
		headers := form.File[rule.Field]
		if len(headers) == 0 {
			fields[rule.Field] = messageFor(r, rule.Field, "required", "")
			continue
		}
		for _, h := range headers {
			if msg := checkOneFile(r, rule, h); msg != "" {
				fields[rule.Field] = msg
				break
			}
		}
	}

	if len(fields) == 0 {
		return nil
	}
	return fields
}

func checkOneFile(r Request, rule FileRule, h *multipart.FileHeader) string {
	if rule.MaxBytes > 0 && h.Size > rule.MaxBytes {
		return messageFor(r, rule.Field, "max", fmt.Sprintf("%d", rule.MaxBytes))
	}
	if rule.MinBytes > 0 && h.Size < rule.MinBytes {
		return messageFor(r, rule.Field, "min", fmt.Sprintf("%d", rule.MinBytes))
	}
	if len(rule.MIMEs) == 0 {
		return ""
	}

	f, err := h.Open()
	if err != nil {
		return messageFor(r, rule.Field, "mimes", strings.Join(rule.MIMEs, ","))
	}
	defer f.Close()

	buf := make([]byte, 512)
	n, _ := io.ReadFull(f, buf)
	detected := http.DetectContentType(buf[:n])
	detected = strings.SplitN(detected, ";", 2)[0]

	for _, allowed := range rule.MIMEs {
		if detected == allowed {
			return ""
		}
	}
	return messageFor(r, rule.Field, "mimes", strings.Join(rule.MIMEs, ","))
}

// BindMultipartCtx parses multipart/form-data, decodes the non-file parts
// into dst, then applies Rules() and FileRules().
func BindMultipartCtx(ctx context.Context, r *http.Request, dst any) error {
	if err := runAuthorizer(ctx, dst); err != nil {
		return err
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return err
	}

	if err := decodeForm(r.MultipartForm.Value, dst); err != nil {
		return err
	}

	if err := ValidateCtx(ctx, dst); err != nil {
		if ve, ok := AsValidationError(err); ok {
			if fm := validateFiles(dst, r.MultipartForm); len(fm) > 0 {
				for k, v := range fm {
					ve.Fields[k] = v
				}
			}
			return ve
		}
		return err
	}

	if fm := validateFiles(dst, r.MultipartForm); len(fm) > 0 {
		return &ValidationError{Fields: fm}
	}
	return nil
}
