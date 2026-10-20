package main

import (
	"fmt"

	"github.com/thomas666-beast/reqval"
)

type StorePostRequest struct {
	reqval.Base

	Title string `validate:"required,max=255"`
	Body  string `validate:"required,min=10"`
}

func main() {
	// Case 1: valid
	good := StorePostRequest{Title: "Hello", Body: "This is long enough"}
	err := reqval.Validate(&good)
	fmt.Println("case 1 (valid):", err)

	// Case 2: invalid on multiple fields
	bad := StorePostRequest{Title: "", Body: "short"}
	err = reqval.Validate(&bad)
	if ve, ok := reqval.AsValidationError(err); ok {
		fmt.Println("case 2 (invalid) status:", ve.HTTPStatus())
		for field, msg := range ve.Fields {
			fmt.Printf("  %s -> %s\n", field, msg)
		}
	} else {
		fmt.Println("case 2 unexpected error:", err)
	}

	// Case 3: max and min together
	tooLong := StorePostRequest{Title: string(make([]byte, 300)), Body: "This is long enough"}
	err = reqval.Validate(&tooLong)
	if ve, ok := reqval.AsValidationError(err); ok {
		fmt.Println("case 3 (too long) status:", ve.HTTPStatus())
		for field, msg := range ve.Fields {
			fmt.Printf("  %s -> %s\n", field, msg)
		}
	}
}
