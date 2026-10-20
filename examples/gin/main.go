package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thomas666-beast/reqval"
)

type StorePostRequest struct {
	reqval.Base

	Title string `json:"title"`
	Body  string `json:"body"`
}

func (r *StorePostRequest) Rules() map[string][]string {
	return map[string][]string{
		"Title": {"required", "max:255"},
		"Body":  {"required", "min:10"},
	}
}

// Messages on the request keep the example self-contained.
// Without this, the fallback fires and the output looks like
// "The Title field is invalid."
func (r *StorePostRequest) Messages() map[string]string {
	return map[string]string{
		"Title.required": "The Title field is required.",
		"Body.min":       "The Body field must be at least {{param}} characters.",
	}
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.POST("/posts", func(c *gin.Context) {
		var req StorePostRequest
		if err := reqval.BindGin(c, &req); err != nil {
			reqval.AbortWithError(c, err)
			return
		}
		c.JSON(201, gin.H{"created": req.Title})
	})

	call(r, "valid", `{"title":"Hello","body":"This is long enough"}`)
	call(r, "invalid", `{"title":"","body":"short"}`)
	call(r, "bad json", `{oops`)
}

func call(r http.Handler, label, body string) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	fmt.Printf("%-10s status=%d response=%s\n", label, w.Code, strings.TrimSpace(w.Body.String()))
}
