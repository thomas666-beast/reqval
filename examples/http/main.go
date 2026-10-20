package main

import (
	"fmt"
	"strings"

	"github.com/thomas666-beast/reqval"
)

type StorePostRequest struct {
	reqval.Base

	Title string `validate:"required,max=255"`
	Body  string `validate:"required,min=10"`
}

func main() {
	cases := []struct {
		name string
		body string
	}{
		{"valid", `{"Title":"Hello","Body":"This is long enough"}`},
		{"missing fields", `{}`},
		{"short body", `{"Title":"Hello","Body":"short"}`},
		{"bad json", `{not json`},
		{"empty", ``},
	}

	for _, tc := range cases {
		var req StorePostRequest
		err := reqval.Bind(strings.NewReader(tc.body), &req)
		switch {
		case err == nil:
			fmt.Printf("%-14s -> OK: %+v\n", tc.name, req)
		case func() bool { _, ok := reqval.AsValidationError(err); return ok }():
			ve, _ := reqval.AsValidationError(err)
			fmt.Printf("%-14s -> 422: %v\n", tc.name, ve.Fields)
		default:
			fmt.Printf("%-14s -> 400: %v\n", tc.name, err)
		}
	}
}
