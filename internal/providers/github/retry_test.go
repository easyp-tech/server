package github

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// failingTripper returns the given error for the first failCount
// requests, then responds with okStatus and okBody. It records the
// number of RoundTrip calls and the method of each.
type failingTripper struct {
	failCount int
	calls     int
	methods   []string
	err       error
	okStatus  int
	okBody    string
}

func (f *failingTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.calls++
	f.methods = append(f.methods, req.Method)
	if f.calls <= f.failCount {
		return nil, f.err
	}
	return &http.Response{
		StatusCode: f.okStatus,
		Body:       io.NopCloser(bytes.NewReader([]byte(f.okBody))),
		Header:     make(http.Header),
	}, nil
}

func TestRetryTransport_RetriesTransientNetErrorThenSucceeds(t *testing.T) {
	// A TLS handshake timeout surfaces as a net.Error whose Timeout()
	// is true — the retryTransport must treat it as transient.
	transient := &timeoutErr{msg: "net/http: TLS handshake timeout", timeout: true}
	base := &failingTripper{failCount: 2, err: transient, okStatus: http.StatusOK, okBody: "ok"}

	rt := &retryTransport{base: base}
	req, _ := http.NewRequest(http.MethodGet, "https://raw.githubusercontent.com/x/y", nil)
	req = req.WithContext(context.Background())

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("expected success after retries, got err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if base.calls != 3 { // 2 failed + 1 success
		t.Fatalf("calls = %d, want 3", base.calls)
	}
	for i, m := range base.methods {
		if m != http.MethodGet {
			t.Fatalf("method[%d] = %q, want GET", i, m)
		}
	}
}

func TestRetryTransport_DoesNotRetryContextCanceled(t *testing.T) {
	base := &failingTripper{failCount: 5, err: context.Canceled, okStatus: http.StatusOK}
	rt := &retryTransport{base: base}
	req, _ := http.NewRequest(http.MethodGet, "https://x", nil)
	req = req.WithContext(context.Background())

	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected the underlying error to be returned")
	}
	// A bare caller-cancel must NOT be retried: exactly one attempt.
	if base.calls != 1 {
		t.Fatalf("calls = %d, want 1 (caller-cancel must not retry)", base.calls)
	}
}

func TestRetryTransport_DoesNotRetryNonIdempotent(t *testing.T) {
	base := &failingTripper{failCount: 1, err: &timeoutErr{timeout: true}, okStatus: http.StatusOK}
	rt := &retryTransport{base: base}
	req, _ := http.NewRequest(http.MethodPost, "https://x", nil)
	req = req.WithContext(context.Background())

	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected the underlying error to be returned (POST not retried)")
	}
	if base.calls != 1 {
		t.Fatalf("calls = %d, want 1 (non-idempotent must not retry)", base.calls)
	}
}

func TestRetryTransport_Retries5xxThenSucceeds(t *testing.T) {
	// First two attempts return 502, third returns 200.
	base := &statusTripper{statuses: []int{http.StatusBadGateway, http.StatusBadGateway, http.StatusOK}}
	rt := &retryTransport{base: base}
	req, _ := http.NewRequest(http.MethodGet, "https://x", nil)
	req = req.WithContext(context.Background())

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if base.calls != 3 {
		t.Fatalf("calls = %d, want 3", base.calls)
	}
}

func TestRetryTransport_GivesUpAfterMaxAttempts(t *testing.T) {
	base := &failingTripper{failCount: 100, err: &timeoutErr{timeout: true}, okStatus: http.StatusOK}
	rt := &retryTransport{base: base}
	req, _ := http.NewRequest(http.MethodGet, "https://x", nil)
	req = req.WithContext(context.Background())

	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if base.calls != retryMaxAttempts {
		t.Fatalf("calls = %d, want %d", base.calls, retryMaxAttempts)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"-3", 0},
		{"2", 2 * time.Second},
		{"120", 120 * time.Second},
		{"not-a-number", 0},
	}
	for _, c := range cases {
		got := parseRetryAfter(c.in)
		if got != c.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestShouldRetryHTTP(t *testing.T) {
	// Transient net error.
	if !shouldRetryHTTP(nil, &timeoutErr{timeout: true}) {
		t.Error("transient net error should retry")
	}
	// Bare caller cancel.
	if shouldRetryHTTP(nil, context.Canceled) {
		t.Error("context.Canceled must not retry")
	}
	// Non-retryable status.
	if shouldRetryHTTP(&http.Response{StatusCode: http.StatusNotFound}, nil) {
		t.Error("404 must not retry")
	}
	// Retryable statuses.
	for code := range map[int]struct{}{
		http.StatusTooManyRequests:     {},
		http.StatusInternalServerError: {},
		http.StatusBadGateway:          {},
		http.StatusServiceUnavailable:  {},
		http.StatusGatewayTimeout:      {},
	} {
		if !shouldRetryHTTP(&http.Response{StatusCode: code}, nil) {
			t.Errorf("status %d should retry", code)
		}
	}
}

// --- helpers ---

var _ net.Error = (*timeoutErr)(nil)

type timeoutErr struct {
	msg     string
	timeout bool
}

func (e *timeoutErr) Error() string   { return e.msg }
func (e *timeoutErr) Timeout() bool   { return e.timeout }
func (e *timeoutErr) Temporary() bool { return e.timeout }

// statusTripper returns a canned status per call index.
type statusTripper struct {
	statuses []int
	calls    int
}

func (s *statusTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	idx := s.calls
	s.calls++
	st := http.StatusOK
	if idx < len(s.statuses) {
		st = s.statuses[idx]
	}
	return &http.Response{
		StatusCode: st,
		Body:       io.NopCloser(bytes.NewReader(nil)),
		Header:     make(http.Header),
	}, nil
}
