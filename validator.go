package reqval

import (
	"sync"
	"github.com/go-playground/validator/v10"
)

var (
	once     sync.Once
	validate *validator.Validate
)

// engine returns the shared validator instance, lazily initialized.
// We use sync.Once so it's safe for concurrent use and cheap to call.
func engine() *validator.Validate {
	once.Do(func() {
		validate = validator.New()
	})
	return validate
}

// Validate runs struct-tag validation on any request struct.
// Returns nil on success, or validator.ValidationErrors on failure.
// Stage 2 will wrap this into a 422-friendly error format.
func Validate(req any) error {
	return engine().Struct(req)
}
