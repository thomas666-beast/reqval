package main

import (
	"fmt"

	"github.com/thomas666-beast/reqval"
)

type RegistrationRequest struct {
	reqval.Base
	Email                string `validate:"required,email"`
	Password             string `validate:"required,min=8,confirmed"`
	PasswordConfirmation string
	Username             string `validate:"required,alpha"`
	Role                 string `validate:"required,in=admin editor viewer"`
	Age                  int    `validate:"required,gte=18,lte=120"`
	Score                int    `validate:"between=1 100"`
	Website              string `validate:"omitempty,url"`
	Code                 string `validate:"omitempty,regexp=^[A-Z]{3}-[0-9]{3}$"`
}

func main() {
	show("valid", &RegistrationRequest{
		Email:                "alice@example.com",
		Password:             "hunter2hunter2",
		PasswordConfirmation: "hunter2hunter2",
		Username:             "alice",
		Role:                 "editor",
		Age:                  30,
		Score:                50,
		Website:              "https://example.com",
		Code:                 "ABC-123",
	})

	show("bad email", &RegistrationRequest{
		Email:                "not-an-email",
		Password:             "hunter2hunter2",
		PasswordConfirmation: "hunter2hunter2",
		Username:             "alice",
		Role:                 "editor",
		Age:                  30,
		Score:                50,
	})

	show("password mismatch", &RegistrationRequest{
		Email:                "alice@example.com",
		Password:             "hunter2hunter2",
		PasswordConfirmation: "different",
		Username:             "alice",
		Role:                 "editor",
		Age:                  30,
		Score:                50,
	})

	show("bad role", &RegistrationRequest{
		Email:                "alice@example.com",
		Password:             "hunter2hunter2",
		PasswordConfirmation: "hunter2hunter2",
		Username:             "alice",
		Role:                 "superuser",
		Age:                  30,
		Score:                50,
	})

	show("under 18", &RegistrationRequest{
		Email:                "alice@example.com",
		Password:             "hunter2hunter2",
		PasswordConfirmation: "hunter2hunter2",
		Username:             "alice",
		Role:                 "editor",
		Age:                  15,
		Score:                50,
	})

	show("bad regex", &RegistrationRequest{
		Email:                "alice@example.com",
		Password:             "hunter2hunter2",
		PasswordConfirmation: "hunter2hunter2",
		Username:             "alice",
		Role:                 "editor",
		Age:                  30,
		Score:                50,
		Code:                 "abc-123",
	})
}

func show(label string, req *RegistrationRequest) {
	err := reqval.Validate(req)
	if err == nil {
		fmt.Printf("%-18s OK\n", label)
		return
	}
	if ve, ok := reqval.AsValidationError(err); ok {
		fmt.Printf("%-18s 422 %v\n", label, ve.Fields)
		return
	}
	fmt.Printf("%-18s err %v\n", label, err)
}
