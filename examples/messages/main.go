package main

import (
	"fmt"

	"github.com/thomas666-beast/reqval"
)

type StorePostRequest struct {
	reqval.Base

	Title string `json:"title"`
	Body  string `json:"body"`
}

func (r *StorePostRequest) Rules() map[string][]string {
	return map[string][]string{
		"Title": {"required", "max:255"},
		"Body":  {"required", "min:10"},
	}
}

func (r *StorePostRequest) Messages() map[string]string {
	return map[string]string{
		"Title.required": "We really need a title.",
		"Body.min":       "The body must be at least {{param}} characters.",
	}
}

type PlainRequest struct {
	reqval.Base

	Name string `json:"name"`
}

func (r *PlainRequest) Rules() map[string][]string {
	return map[string][]string{
		"Name": {"required"},
	}
}

func main() {
	show("fallback (no bundle)", &PlainRequest{})
	reqval.SetDefaultMessages(reqval.ZH())
	show("app bundle (ZH)", &PlainRequest{})
	show("request override wins", &StorePostRequest{})
}

func show(label string, req reqval.Request) {
	err := reqval.Validate(req)
	if err == nil {
		fmt.Printf("%-25s OK\n", label)
		return
	}
	if ve, ok := reqval.AsValidationError(err); ok {
		fmt.Printf("%-25s 422 %v\n", label, ve.Fields)
		return
	}
	fmt.Printf("%-25s err %v\n", label, err)
}
