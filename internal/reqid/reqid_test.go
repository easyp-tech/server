package reqid_test

import (
	"context"
	"testing"

	"github.com/easyp-tech/server/internal/reqid"
)

func TestRoundTrip(t *testing.T) {
	const id = "abc123"
	ctx := reqid.With(context.Background(), id)
	if got := reqid.From(ctx); got != id {
		t.Fatalf("From(With(bg, %q)) = %q, want %q", id, got, id)
	}
}

func TestFromEmpty(t *testing.T) {
	if got := reqid.From(context.Background()); got != "" {
		t.Fatalf("From(bg) = %q, want empty", got)
	}
}
