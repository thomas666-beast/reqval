package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/thomas666-beast/reqval"
)

type ctxKey int

const actorKey ctxKey = 1

type Actor struct {
	ID   int64
	Role string
}

func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey, a)
}

func ActorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey).(Actor)
	return a, ok
}

type StorePostRequest struct {
	reqval.Base
	Title string `json:"title" validate:"required,max=255"`
}

func (r *StorePostRequest) Authorize(ctx context.Context) error {
	a, ok := ActorFrom(ctx)
	if !ok {
		return fmt.Errorf("unauthenticated")
	}
	if a.Role != "admin" {
		return fmt.Errorf("only admins can create posts")
	}
	return nil
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
		c.JSON(201, gin.H{"created": req.Title, "by": "admin"})
	})

	call(r, "no actor, valid body", "", `{"title":"Hello"}`)
	call(r, "editor, valid body", "editor", `{"title":"Hello"}`)
	call(r, "admin, valid body", "admin", `{"title":"Hello"}`)
	call(r, "admin, invalid body", "admin", `{"title":""}`)
}

func call(h http.Handler, label, role, body string) {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		ctx := WithActor(req.Context(), Actor{ID: 1, Role: role})
		req = req.WithContext(ctx)
	}
	h.ServeHTTP(w, req)
	fmt.Printf("%-22s status=%d body=%s\n", label, w.Code, strings.TrimSpace(w.Body.String()))
}
