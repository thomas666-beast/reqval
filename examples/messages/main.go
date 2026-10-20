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

func show(label string, req *StorePostRequest) {
	err := reqval.Validate(req)
	if ve, ok := reqval.AsValidationError(err); ok {
		fmt.Printf("%s:\n", label)
		for _, k := range []string{"Title", "Body"} {
			if m, ok := ve.Fields[k]; ok {
				fmt.Printf("  %s -> %s\n", k, m)
			}
		}
	}
}

func main() {
	bad := &StorePostRequest{}

	show("1) default EN", bad)

	reqval.RegisterMessage("required", "Please fill in {{field}}.")
	show("2) tag override", bad)

	reqval.RegisterFieldMessage("StorePostRequest", "Title", "We really need a title.")
	show("3) field override (Title)", bad)

	reqval.LoadZH()
	show("4) ZH bundle (Title override still wins)", bad)
}
