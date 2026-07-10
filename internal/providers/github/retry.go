package github

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"time"
)

// retryMaxAttempts is the maximum number of HTTP round-trips the
// retryTransport will issue for a single request (initial attempt +
// retries). Each GitHub read (GetTree, DownloadContents →
// raw.githubusercontent.com) fans out into many individual file GETs;
// a single transient TLS handshake timeout must not fail the whole
// batch, so we retry transient failures a bounded number of times.
const retryMaxAttempts = 4

// retryBaseBackoff is the delay before the first retry; subsequent
// retries back off exponentially (base * 2^(attempt-1)) with jitter,
// capped at retryMaxBackoff.
const (
	retryBaseBackoff = 500 * time.Millisecond
	retryMaxBackoff  = 8 * time.Second
)

// retryTransport wraps an http.RoundTripper and replays safe read
// requests that fail with transient network errors (TLS handshake
// timeout, EOF, connection reset, deadline exceeded) or return
// retryable HTTP statuses (429, 500, 502, 503, 504). It never retries
// non-idempotent methods or request-level context cancellations
// (those are the caller's intent, not transient upstream flakiness).
type retryTransport struct {
	base http.RoundTripper
	log  *slog.Logger
}

func (t *retryTransport) transport() http.RoundTripper {
	if t.base != nil {
		return t.base
	}
	return http.DefaultTransport
}

// RoundTrip implements http.RoundTripper with bounded retry.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only replay idempotent reads. All GitHub provider upstream calls
	// here are GETs (tree listing + content download).
	if !isIdempotent(req.Method) {
		return t.transport().RoundTrip(req)
	}

	// Snapshot the request body so each replay can re-send it. GitHub
	// reads have no body, but the transport is generic.
	var bodySnapshot []byte
	if req.Body != nil && req.GetBody != nil {
		// Prefer GetBody (the stdlib's replay-friendly hook) when present.
	} else if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		_ = req.Body.Close()
		bodySnapshot = b
		req.Body = io.NopCloser(bytes.NewReader(b))
	}

	var (
		resp    *http.Response
		err     error
		attempt int
	)
	for attempt = 1; attempt <= retryMaxAttempts; attempt++ {
		if attempt > 1 && bodySnapshot != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodySnapshot))
		}

		resp, err = t.transport().RoundTrip(req)

		if !shouldRetryHTTP(resp, err) {
			return resp, err
		}

		// Discard any partial response body so the underlying connection
		// can be reused and we do not leak.
		if resp != nil {
			if resp.Body != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			}
			resp = nil
		}

		// Do not sleep after the final attempt.
		if attempt == retryMaxAttempts {
			break
		}

		backoff := nextBackoff(attempt, resp)
		if t.log != nil {
			t.log.LogAttrs(context.Background(), slog.LevelDebug, "github upstream retry",
				slog.String("method", req.Method),
				slog.String("url", req.URL.String()),
				slog.Int("attempt", attempt),
				slog.Duration("backoff", backoff),
				slog.String("error", errString(err)),
			)
		}

		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(backoff):
		}
	}

	// Exhausted retries: return the last error/response unchanged so
	// callers see the real failure, not a retry artifact.
	return resp, err
}

// isIdempotent reports whether method is safe to replay without
// side effects.
func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// shouldRetryHTTP returns true for transient network errors and
// retryable status codes. A request-level context cancellation
// (context.Canceled originating from the caller) is NOT retried — it
// is intentional. context.DeadlineExceeded IS retried because it
// typically wraps a transient per-call timeout.
func shouldRetryHTTP(resp *http.Response, err error) bool {
	if err != nil {
		if errors.Is(err, context.Canceled) {
			// Caller cancelled: never retry.
			// (A context.Canceled nested inside a net.Error is still
			// surfaced as the net error; this branch catches the bare
			// caller-cancel case.)
			return false
		}
		// Any other network/transport error (TLS handshake timeout,
		// EOF, connection reset/refused, net.Error timeout) is
		// transient from the upstream's perspective.
		return true
	}
	if resp == nil {
		return false
	}
	switch resp.StatusCode {
	case http.StatusTooManyRequests, // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout:      // 504
		return true
	default:
		return false
	}
}

// nextBackoff computes the delay before the next attempt. It honors a
// Retry-After header when present (429/503), otherwise uses
// exponential backoff with jitter, capped at retryMaxBackoff.
func nextBackoff(attempt int, resp *http.Response) time.Duration {
	if resp != nil {
		if ra := parseRetryAfter(resp.Header.Get("Retry-After")); ra > 0 {
			if ra > retryMaxBackoff {
				return retryMaxBackoff
			}
			return ra
		}
	}
	// Exponential: base * 2^(attempt-1), capped, + up to 30% jitter.
	backoff := retryBaseBackoff << (attempt - 1)
	if backoff <= 0 || backoff > retryMaxBackoff {
		backoff = retryMaxBackoff
	}
	jitter := time.Duration(rand.Int64N(int64(backoff) / 3))
	return backoff + jitter
}

// parseRetryAfter parses an HTTP Retry-After header expressed either as
// delta-seconds or an RFC1123 date. Returns 0 if unparseable.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	// Delta-seconds (the common case for GitHub 429/503).
	if seconds, err := strconv.Atoi(v); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	// RFC1123 date.
	t, err := time.Parse(time.RFC1123, v)
	if err != nil {
		return 0
	}
	d := time.Until(t)
	if d < 0 {
		return 0
	}
	return d
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	// Surface the innermost network detail for logs without leaking
	// the full stack.
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Error()
	}
	return err.Error()
}
