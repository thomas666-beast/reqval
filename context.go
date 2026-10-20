package reqval

import "context"

type ctxKey int

const ignoreIDKey ctxKey = 1

// WithIgnoreID attaches a row ID to exclude from unique checks.
// Use on update requests: unique=users:username:id will skip the row with this ID.
func WithIgnoreID(ctx context.Context, id any) context.Context {
	return context.WithValue(ctx, ignoreIDKey, id)
}

// ignoreIDFrom extracts the ignore ID from a context, if set.
func ignoreIDFrom(ctx context.Context) (any, bool) {
	v := ctx.Value(ignoreIDKey)
	return v, v != nil
}
