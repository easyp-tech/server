package artifactory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/easyp-tech/server/internal/providers/content"
)

func TestPut_RejectsErrorStatusCodes(t *testing.T) {
	// Create test server that returns error status
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden) // 403
	}))
	defer server.Close()

	// Create artifactory client pointing to test server
	c := New(
		nil,
		server.URL+"/",
		"user",
		"pass",
		0,
		0,
	)

	// Call Put - should return error for 403
	err := c.Put(context.Background(), "owner", "repo", "commit", "hash", []content.File{})
	if err == nil {
		t.Errorf("Put() should return error for status 403")
	}
}

func TestPut_AcceptsSuccessStatusCodes(t *testing.T) {
	// Create test server that returns success
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	c := New(nil, server.URL+"/", "user", "pass", 0, 0)

	err := c.Put(context.Background(), "owner", "repo", "commit", "hash", []content.File{})
	if err != nil {
		t.Errorf("Put() should not return error for status 200, got: %v", err)
	}
}

func TestGet_ReturnsNilFor404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	c := New(nil, server.URL+"/", "user", "pass", 0, 0)

	files, err := c.Get(context.Background(), "owner", "repo", "commit", "hash")
	if err != nil {
		t.Errorf("Get() should not return error for 404, got: %v", err)
	}
	if files != nil {
		t.Errorf("Get() should return nil for 404 cache miss")
	}
}