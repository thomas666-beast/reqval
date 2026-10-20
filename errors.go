package reqval

import (
	"errors"
	"fmt"
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

func (e *ValidationError) HTTPStatus() int { return 422 }

func AsValidationError(err error) (*ValidationError, bool) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}

type AuthorizationError struct {
	Message string
}

func (e *AuthorizationError) Error() string {
	if e.Message == "" {
		return "forbidden"
	}
	return e.Message
}

func (e *AuthorizationError) HTTPStatus() int { return 403 }

func AsAuthorizationError(err error) (*AuthorizationError, bool) {
	var ae *AuthorizationError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}
