package main

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"

	"github.com/gin-gonic/gin"
	"github.com/thomas666-beast/reqval"
)

type UploadRequest struct {
	reqval.Base
	Description string `form:"description"`
}

func (r *UploadRequest) Rules() map[string][]string {
	return map[string][]string{"Description": {"required", "max:200"}}
}

func (r *UploadRequest) FileRules() []reqval.FileRule {
	return []reqval.FileRule{
		{Field: "avatar", MaxBytes: 2 << 20, MIMEs: []string{"image/png", "image/jpeg"}},
	}
}

func (r *UploadRequest) Messages() map[string]string {
	return map[string]string{
		"avatar.mimes": "Avatar must be a PNG or JPEG.",
	}
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.POST("/upload", func(c *gin.Context) {
		var req UploadRequest
		if err := reqval.BindMultipartGin(c, &req); err != nil {
			reqval.AbortWithError(c, err)
			return
		}
		c.JSON(201, gin.H{"description": req.Description})
	})

	// valid PNG upload
	call(g, "valid png", "my avatar", pngBytes(), "avatar.png", "image/png")
	// wrong mime
	call(g, "wrong mime", "my avatar", []byte("plain text"), "avatar.txt", "text/plain")
	// missing file
	call(g, "missing file", "my avatar", nil, "", "")
	// missing description
	call(g, "missing description", "", pngBytes(), "avatar.png", "image/png")
}

func call(h http.Handler, label, desc string, file []byte, filename, ctype string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	_ = w.WriteField("description", desc)
	if filename != "" {
		fw, _ := w.CreateFormFile("avatar", filename)
		fw.Write(file)
	}
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	fmt.Printf("%-22s status=%d body=%s\n", label, rec.Code, rec.Body.String())
}

// minimal valid PNG magic bytes
func pngBytes() []byte {
	return []byte{
		0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A,
		0, 0, 0, 0, 0, 0, 0, 0,
	}
}
