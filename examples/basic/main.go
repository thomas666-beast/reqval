package main

import (
	"fmt"

	"github.com/thomas666-beast/reqval"
)

// StorePostRequest mirrors a Laravel Form Request.
type StorePostRequest struct {
	reqval.Base // satisfies reqval.Request; marker only in Stage 1

	Title string `validate:"required,max=255"`
	Body  string `validate:"required"`
}

func main() {
	// Case 1: valid payload
	good := StorePostRequest{
		Title: "Hello",
		Body:  "World",
	}
	fmt.Println("case 1 (valid):", reqval.Validate(&good))

	// Case 2: missing fields
	bad := StorePostRequest{}
	fmt.Println("case 2 (invalid):", reqval.Validate(&bad))

	// Case 3: title too long
	long := StorePostRequest{
		Title: string(make([]byte, 300)), // 300 zero bytes > max=255
		Body:  "x",
	}
	fmt.Println("case 3 (too long):", reqval.Validate(&long))
}
