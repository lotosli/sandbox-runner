package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMakeDirsEncodesModesAsOctalDigitIntegers(t *testing.T) {
	var got map[string]map[string]int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sandboxes/sbx-1/endpoints/44772":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"endpoint": contractURLWithoutScheme(r),
				"headers":  map[string]string{"X-EXECD-ACCESS-TOKEN": "token-1"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sbx-1/proxy/44772/directories":
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Fatalf("Decode(request body) error = %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(Config{BaseURL: server.URL})
	if err := client.MakeDirs(context.Background(), "sbx-1", map[string]int{"/workspace": 0o755}); err != nil {
		t.Fatalf("MakeDirs() error = %v", err)
	}
	if got["/workspace"]["mode"] != 755 {
		t.Fatalf("mode = %d, want 755", got["/workspace"]["mode"])
	}
}

func contractURLWithoutScheme(r *http.Request) string {
	return r.Host + "/sandboxes/sbx-1/proxy/44772"
}
