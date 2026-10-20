package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thomas666-beast/reqval"
)

type LoginRequest struct {
	reqval.Base
	Email    string   `form:"email"`
	Password string   `form:"password"`
	Roles    []string `form:"roles"`
}

func (r *LoginRequest) Rules() map[string][]string {
	return map[string][]string{
		"Email":    {"required", "email"},
		"Password": {"required", "min:8"},
		"Roles":    {"required", "min:1"},
	}
}

func (r *LoginRequest) Messages() map[string]string {
	return map[string]string{
		"Email.email": "That is not a valid email.",
	}
}

func main() {
	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.POST("/login", func(c *gin.Context) {
		var req LoginRequest
		if err := reqval.BindFormGin(c, &req); err != nil {
			reqval.AbortWithError(c, err)
			return
		}
		c.JSON(200, gin.H{"email": req.Email, "roles": req.Roles})
	})

	call(g, "valid",
		url.Values{"email": {"a@b.com"}, "password": {"hunter2hunter"}, "roles": {"admin", "editor"}})
	call(g, "bad email",
		url.Values{"email": {"nope"}, "password": {"hunter2hunter"}, "roles": {"admin"}})
	call(g, "short password",
		url.Values{"email": {"a@b.com"}, "password": {"x"}, "roles": {"admin"}})
	call(g, "no roles",
		url.Values{"email": {"a@b.com"}, "password": {"hunter2hunter"}})
}

func call(h http.Handler, label string, form url.Values) {
	body := strings.NewReader(form.Encode())
	req := httptest.NewRequest(http.MethodPost, "/login", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	fmt.Printf("%-16s status=%d body=%s\n", label, rec.Code, rec.Body.String())
}
