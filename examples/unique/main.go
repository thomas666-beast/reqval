package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/thomas666-beast/reqval"
)

type CreateUserRequest struct {
	reqval.Base
	Username string `validate:"required,unique=users:username"`
}

type UpdateUserRequest struct {
	reqval.Base
	Username string `validate:"required,unique=users:username:id:42"`
}

type UpdateUserRequestNoIgnore struct {
	reqval.Base
	Username string `validate:"required,unique=users:username:id:"`
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

	// --- Create path ---
	fmt.Println("create valid (new user):",
		reqval.Validate(&CreateUserRequest{Username: "carol"}))
	printErr("create invalid (alice exists)",
		reqval.Validate(&CreateUserRequest{Username: "alice"}))

	// --- Update path (ignore id=42, the row we're editing) ---
	fmt.Println("update: keep own username 'alice' ->",
		reqval.Validate(&UpdateUserRequest{Username: "alice"}))
	printErr("update: take 'bob' (owned by 43) ->",
		reqval.Validate(&UpdateUserRequest{Username: "bob"}))
	fmt.Println("update: change to new 'dave' ->",
		reqval.Validate(&UpdateUserRequest{Username: "dave"}))

	// --- Optional ignoreID (empty) degrades to plain unique ---
	printErr("update no-ignore: 'alice' exists ->",
		reqval.Validate(&UpdateUserRequestNoIgnore{Username: "alice"}))
	fmt.Println("update no-ignore: 'erin' is new ->",
		reqval.Validate(&UpdateUserRequestNoIgnore{Username: "erin"}))
}

func mustExec(db *sql.DB, q string) {
	if _, err := db.Exec(q); err != nil {
		panic(err)
	}
}

func printErr(label string, err error) {
	if ve, ok := reqval.AsValidationError(err); ok {
		fmt.Printf("%s 422: %v\n", label, ve.Fields)
		return
	}
	fmt.Printf("%s err: %v\n", label, err)
}
