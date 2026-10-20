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

	Title string `json:"title" validate:"required,max=255"`
	Body  string `json:"body" validate:"required,min=10"`
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	r.POST("/posts", func(c *gin.Context) {
		var req StorePostRequest
		if err := reqval.BindGin(c, &req); err != nil {
			reqval.AbortWithValidationError(c, err)
			return
		}
		c.JSON(201, gin.H{"created": req.Title})
	})

	// Case 1: valid
	call(r, `{"title":"Hello","body":"This is long enough"}`)

	// Case 2: validation failure
	call(r, `{"title":"","body":"short"}`)

	// Case 3: bad JSON
	call(r, `{oops`)
}

func call(r http.Handler, body string) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	fmt.Printf("body=%-40s status=%d response=%s\n", body, w.Code, strings.TrimSpace(w.Body.String()))
}
