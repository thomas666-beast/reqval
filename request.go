package reqval

import "context"

// Request is implemented by every request type.
// Rules and Messages are the per-form configuration;
// Authorize is the pre-validation access check.
type Request interface {
	Rules() map[string][]string
	Messages() map[string]string
	Authorize(ctx context.Context) error
}

// Base is embedded by request structs that only need defaults.
// It provides empty Rules/Messages/Authorize so a struct can be
// written incrementally.
type Base struct{}

func (Base) Rules() map[string][]string         { return nil }
func (Base) Messages() map[string]string         { return nil }
func (Base) Authorize(ctx context.Context) error { return nil }
