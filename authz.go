package reqval

import "context"

// runAuthorizer calls Authorize if the request implements Request.
func runAuthorizer(ctx context.Context, req any) error {
	r, ok := req.(Request)
	if !ok {
		return nil
	}
	if err := r.Authorize(ctx); err != nil {
		return &AuthorizationError{Message: err.Error()}
	}
	return nil
}
