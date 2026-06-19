// Package reqid stores and retrieves the per-request correlation ID used
// across logging boundaries. It exists as a tiny, dependency-free package so
// that lower-level packages (e.g. internal/providers/multisource) can read
// the request_id from context without importing internal/connect, which
// would form an import cycle (connect depends on multisource via the
// provider interface).
package reqid

import "context"

// ctxKey is unexported so callers cannot construct a value from outside the
// package; the only way to put a request_id into a context is via With.
type ctxKey struct{}

// With returns a copy of ctx that carries the given request id.
func With(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// From returns the request id stored in ctx, or empty string if absent.
// Safe to call from any package; the unexported ctxKey prevents collisions.
func From(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKey{}).(string); ok {
		return id
	}
	return ""
}
