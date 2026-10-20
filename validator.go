package reqval

import (
	"context"
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	once     sync.Once
	validate *validator.Validate
)

func engine() *validator.Validate {
	once.Do(func() {
		validate = validator.New()
		registerRules(validate) // pass instance, do NOT re-enter engine()
	})
	return validate
}

func Validate(req any) error {
	return ValidateCtx(context.Background(), req)
}

func ValidateCtx(ctx context.Context, req any) error {
	if err := engine().StructCtx(ctx, req); err != nil {
		return translate(err)
	}
	return nil
}
