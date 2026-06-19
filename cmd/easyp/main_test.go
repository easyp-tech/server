package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestLoggingMiddleware_CapturesImplicit200 pins the fix for "status:0" in
// access logs. Handlers in internal/connect call Header().Set / Write
// without an explicit WriteHeader; net/http then sends an implicit 200 OK
// to the client. The middleware wrapper must reflect that 200 into the
// logged status, not the zero value.
//
// Regression test for the bug surfaced in the user's installation logs.
func TestLoggingMiddleware_CapturesImplicit200(t *testing.T) {
	var buf strings.Builder
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	// A handler that writes a body without ever calling WriteHeader —
	// exactly the pattern used by the raw HTTP handlers in internal/connect.
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		_, _ = w.Write([]byte{0x08, 0x01}) // arbitrary bytes
	})

	mw := loggingMiddleware(log, "test.example.com", next)
	server := httptest.NewServer(mw)
	defer server.Close()

	resp, err := http.Post(server.URL+"/anything", "application/proto", strings.NewReader(""))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("wire status = %d, want 200", resp.StatusCode)
	}

	logLine := buf.String()
	if !strings.Contains(logLine, `"status":200`) {
		t.Errorf("access log does not contain status:200\nlog: %s", logLine)
	}
	if strings.Contains(logLine, `"status":0`) {
		t.Errorf("access log still contains status:0\nlog: %s", logLine)
	}
}

// TestLoggingMiddleware_RecordsExplicitStatus covers the opposite case:
// when the handler does call WriteHeader, the wrapper must capture that
// exact code (not overwrite it with 200 on the subsequent Write).
func TestLoggingMiddleware_RecordsExplicitStatus(t *testing.T) {
	var buf strings.Builder
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("coffee"))
	})

	mw := loggingMiddleware(log, "test.example.com", next)
	server := httptest.NewServer(mw)
	defer server.Close()

	resp, err := http.Get(server.URL + "/anything")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusTeapot {
		t.Fatalf("wire status = %d, want 418", resp.StatusCode)
	}
	if !strings.Contains(buf.String(), `"status":418`) {
		t.Errorf("access log does not contain status:418\nlog: %s", buf.String())
	}
}

// TestLoggingMiddleware_EmptyBodyStillReports200 covers the empty-graph /
// empty-modules case: w.Write(nil) must still result in status:200, not 0.
// (Real handlers in internal/connect do call w.Write(nil) for empty
// results, so this is an actual production path.)
func TestLoggingMiddleware_EmptyBodyStillReports200(t *testing.T) {
	var buf strings.Builder
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/proto")
		_, _ = w.Write(nil) // w.Write(nil) — explicit empty body, no WriteHeader
	})

	mw := loggingMiddleware(log, "test.example.com", next)
	server := httptest.NewServer(mw)
	defer server.Close()

	resp, err := http.Post(server.URL+"/anything", "application/proto", strings.NewReader(""))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("wire status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(buf.String(), `"status":200`) {
		t.Errorf("access log does not contain status:200\nlog: %s", buf.String())
	}
}
