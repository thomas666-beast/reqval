package reqval

import "context"

type ctxKey int

const ignoreIDKey ctxKey = 1

func WithIgnoreID(ctx context.Context, id any) context.Context {
	return context.WithValue(ctx, ignoreIDKey, id)
}

func ignoreIDFrom(ctx context.Context) (any, bool) {
	v := ctx.Value(ignoreIDKey)
	return v, v != nil
}
