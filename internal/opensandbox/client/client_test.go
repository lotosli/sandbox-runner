package client

import "testing"

func TestNormalizeEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		endpoint string
		want     string
	}{
		{name: "keeps absolute URL", baseURL: "http://127.0.0.1:8080", endpoint: "https://execd.example/proxy", want: "https://execd.example/proxy"},
		{name: "adds scheme from base URL", baseURL: "https://127.0.0.1:8080", endpoint: "127.0.0.1:9000/proxy/44772", want: "https://127.0.0.1:9000/proxy/44772"},
		{name: "handles scheme-relative endpoint", baseURL: "http://127.0.0.1:8080", endpoint: "//127.0.0.1:9000/proxy", want: "http://127.0.0.1:9000/proxy"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeEndpoint(tc.baseURL, tc.endpoint); got != tc.want {
				t.Fatalf("normalizeEndpoint(%q, %q) = %q, want %q", tc.baseURL, tc.endpoint, got, tc.want)
			}
		})
	}
}
