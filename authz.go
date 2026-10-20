package reqval

import (
	"context"
	"errors"
)

// Authorizer is optionally implemented by request structs that need
// pre-validation access checks (Laravel's FormRequest::authorize()).
type Authorizer interface {
	Authorize(ctx context.Context) error
}

// AuthorizationError signals that the actor is not allowed.
// Maps to HTTP 403, distinct from 422 validation errors.
type AuthorizationError struct {
	Message string
}

func (e *AuthorizationError) Error() string {
	if e.Message == "" {
		return "forbidden"
	}
	return e.Message
}

func (e *AuthorizationError) HTTPStatus() int {
	return 403
}

// AsAuthorizationError unwraps any error into *AuthorizationError if possible.
func AsAuthorizationError(err error) (*AuthorizationError, bool) {
	var ae *AuthorizationError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

// runAuthorizer calls Authorize if dst implements Authorizer.
// Returns *AuthorizationError on denial, nil otherwise.
func runAuthorizer(ctx context.Context, dst any) error {
	a, ok := dst.(Authorizer)
	if !ok {
		return nil
	}
	if err := a.Authorize(ctx); err != nil {
		if _, isAuthz := AsAuthorizationError(err); isAuthz {
			return err
		}
		return &AuthorizationError{Message: err.Error()}
	}
	return nil
}
