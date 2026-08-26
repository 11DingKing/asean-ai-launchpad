package requestctx

import "context"

type key string

const (
	requestIDKey key = "request-id"
	principalKey key = "principal"
)

type Principal struct {
	UserID string
	Role   string
}

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalKey, principal)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	value, ok := ctx.Value(principalKey).(Principal)
	return value, ok
}
