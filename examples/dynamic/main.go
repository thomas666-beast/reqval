package main

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/thomas666-beast/reqval"
)

// One struct, used for both create and update.
type UserRequest struct {
	reqval.Base
	Username string `validate:"required,unique=users:username:id"`
}

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()
	mustExec(db, `CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT UNIQUE)`)
	mustExec(db, `INSERT INTO users (id, username) VALUES (42, 'alice'), (43, 'bob')`)
	reqval.RegisterDB(db)

	// CREATE flow: no ignore ID in ctx, alice and bob are taken.
	show("create: carol (new)", reqval.ValidateCtx(context.Background(),
		&UserRequest{Username: "carol"}))
	show("create: alice (taken)", reqval.ValidateCtx(context.Background(),
		&UserRequest{Username: "alice"}))

	// UPDATE user 42: keep own username (must pass), take bob's (must fail).
	ctxUpdate42 := reqval.WithIgnoreID(context.Background(), int64(42))
	show("update#42: keep 'alice'", reqval.ValidateCtx(ctxUpdate42,
		&UserRequest{Username: "alice"}))
	show("update#42: take 'bob'", reqval.ValidateCtx(ctxUpdate42,
		&UserRequest{Username: "bob"}))
	show("update#42: change to 'carol'", reqval.ValidateCtx(ctxUpdate42,
		&UserRequest{Username: "carol"}))
}

func show(label string, err error) {
	if err == nil {
		fmt.Printf("%-26s OK\n", label)
		return
	}
	if ve, ok := reqval.AsValidationError(err); ok {
		fmt.Printf("%-26s 422 %v\n", label, ve.Fields)
		return
	}
	fmt.Printf("%-26s err %v\n", label, err)
}

func mustExec(db *sql.DB, q string) {
	if _, err := db.Exec(q); err != nil {
		panic(err)
	}
}
