package reqval

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed on %d field(s)", len(e.Fields))
}

func (e *ValidationError) HTTPStatus() int {
	return 422
}

func AsValidationError(err error) (*ValidationError, bool) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}

func translate(err error) *ValidationError {
	var vErrs validator.ValidationErrors
	if !errors.As(err, &vErrs) {
		return &ValidationError{Fields: map[string]string{"_error": err.Error()}}
	}

	fields := make(map[string]string, len(vErrs))
	for _, fe := range vErrs {
		fields[fe.Field()] = resolve(structNameFrom(fe), fe)
	}
	return &ValidationError{Fields: fields}
}

// structNameFrom returns the parent struct name from StructNamespace().
// e.g. "StorePostRequest.Title" -> "StorePostRequest".
func structNameFrom(fe validator.FieldError) string {
	ns := fe.StructNamespace()
	if i := strings.Index(ns, "."); i > 0 {
		return ns[:i]
	}
	return ns
}
