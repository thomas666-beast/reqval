package main

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/thomas666-beast/reqval"
)

type StorePostRequest struct {
	reqval.Base

	Title      string `validate:"required,max=255"`
	CategoryID int64  `validate:"required,exists=categories:id"`
}

type EmptyCategory struct {
	reqval.Base

	CategoryID int64 `validate:"exists=categories:id"`
}

func main() {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		panic(err)
	}
	defer db.Close()

	mustExec(db, `CREATE TABLE categories (id INTEGER PRIMARY KEY, name TEXT)`)
	mustExec(db, `INSERT INTO categories (id, name) VALUES (1, 'go'), (2, 'php')`)

	reqval.RegisterDB(db)

	// Case 1: valid — category 1 exists
	ok := StorePostRequest{Title: "Hello", CategoryID: 1}
	fmt.Println("case 1 (valid):", reqval.Validate(&ok))

	// Case 2: category 99 doesn't exist
	bad := StorePostRequest{Title: "Hello", CategoryID: 99}
	printErr("case 2 (category missing)", reqval.Validate(&bad))

	// Case 3: missing CategoryID (0) -> required fires, exists skips
	missing := StorePostRequest{Title: "Hello"}
	printErr("case 3 (required, not exists)", reqval.Validate(&missing))

	// Case 4: only exists on the field, value is 0 -> exists skips, whole thing passes
	fmt.Println("case 4 (exists skipped on zero):", reqval.Validate(&EmptyCategory{}))

	// Case 5: exists on the field, valid value
	fmt.Println("case 5 (exists passes on 2):", reqval.Validate(&EmptyCategory{CategoryID: 2}))
}

func mustExec(db *sql.DB, q string) {
	if _, err := db.Exec(q); err != nil {
		panic(err)
	}
}

func printErr(label string, err error) {
	if ve, ok := reqval.AsValidationError(err); ok {
		fmt.Printf("%s -> 422: %v\n", label, ve.Fields)
		return
	}
	fmt.Printf("%s -> err: %v\n", label, err)
}
