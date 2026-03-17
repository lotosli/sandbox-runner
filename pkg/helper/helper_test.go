package helper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunAttrsFromEnv(t *testing.T) {
	t.Setenv("RUN_ID", "r-1")
	t.Setenv("ATTEMPT", "2")
	t.Setenv("SANDBOX_ID", "sbx-1")
	attrs := RunAttrsFromEnv()
	if len(attrs) != 3 {
		t.Fatalf("expected 3 attrs, got %d", len(attrs))
	}
}

func TestWrapHTTPHandler(t *testing.T) {
	handler := WrapHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest(http.MethodGet, "/health", nil).WithContext(context.Background())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
