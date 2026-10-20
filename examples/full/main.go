package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/gin-gonic/gin"
	_ "modernc.org/sqlite"

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

func actorFrom(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey).(Actor)
	return a, ok
}

type StorePostRequest struct {
	reqval.Base
	Title      string `json:"title" validate:"required,max=255"`
	Slug       string `json:"slug" validate:"required,unique=posts:slug"`
	CategoryID int64  `json:"category_id" validate:"required,exists=categories:id"`
}

func (r *StorePostRequest) Authorize(ctx context.Context) error {
	a, ok := actorFrom(ctx)
	if !ok {
		return fmt.Errorf("unauthenticated")
	}
	if a.Role != "admin" {
		return fmt.Errorf("admin only")
	}
	return nil
}

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	mustExec(db, `CREATE TABLE posts (id INTEGER PRIMARY KEY, slug TEXT UNIQUE)`)
	mustExec(db, `INSERT INTO posts (slug) VALUES ('existing-post')`)
	mustExec(db, `CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT)`)
	mustExec(db, `INSERT INTO categories (id, name) VALUES (1, 'go')`)

	reqval.RegisterDB(db)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.POST("/posts", func(c *gin.Context) {
		var req StorePostRequest
		if err := reqval.BindGin(c, &req); err != nil {
			reqval.AbortWithError(c, err)
			return
		}
		c.JSON(201, gin.H{"created": req.Title, "slug": req.Slug, "category_id": req.CategoryID})
	})

	fmt.Println("== full end-to-end ==")
	call(r, "no actor", "", `{"title":"Hi","slug":"new","category_id":1}`)
	call(r, "editor", "editor", `{"title":"Hi","slug":"new","category_id":1}`)
	call(r, "admin, all valid", "admin", `{"title":"Hi","slug":"new","category_id":1}`)
	call(r, "admin, dup slug", "admin", `{"title":"Hi","slug":"existing-post","category_id":1}`)
	call(r, "admin, bad category", "admin", `{"title":"Hi","slug":"new2","category_id":99}`)
	call(r, "admin, empty title", "admin", `{"title":"","slug":"new3","category_id":1}`)
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
	fmt.Printf("%-22s %d %s\n", label, w.Code, strings.TrimSpace(w.Body.String()))
}

func mustExec(db *sql.DB, q string) {
	if _, err := db.Exec(q); err != nil {
		panic(err)
	}
}
