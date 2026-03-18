package kubernetes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

func TestClientCreateJobUsesMinimalRESTRequest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/apis/batch/v1/namespaces/sandbox-system/jobs" {
			t.Fatalf("path = %s, want /apis/batch/v1/namespaces/sandbox-system/jobs", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q, want Bearer test-token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("content-type = %q, want application/json", got)
		}
		var job batchv1.Job
		if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if job.Name != "demo-job" {
			t.Fatalf("job name = %s, want demo-job", job.Name)
		}
		_ = json.NewEncoder(w).Encode(batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "sandbox-system",
				Name:      "demo-job",
				Labels:    map[string]string{"app": "sandbox-runner"},
			},
		})
	}))
	defer server.Close()

	restClient, err := newClientFromRESTConfig(&rest.Config{
		Host:        server.URL,
		BearerToken: "test-token",
		Timeout:     5 * time.Second,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	})
	if err != nil {
		t.Fatalf("newClientFromRESTConfig() error = %v", err)
	}

	created, err := restClient.CreateJob(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "sandbox-system",
			Name:      "demo-job",
		},
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if created.Namespace != "sandbox-system" {
		t.Fatalf("namespace = %s, want sandbox-system", created.Namespace)
	}
	if created.Name != "demo-job" {
		t.Fatalf("name = %s, want demo-job", created.Name)
	}
}

func TestClientCreateJobReportsServerErrors(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
	}))
	defer server.Close()

	restClient, err := newClientFromRESTConfig(&rest.Config{
		Host:    server.URL,
		Timeout: 5 * time.Second,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	})
	if err != nil {
		t.Fatalf("newClientFromRESTConfig() error = %v", err)
	}

	_, err = restClient.CreateJob(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "sandbox-system",
			Name:      "demo-job",
		},
	})
	if err == nil {
		t.Fatal("CreateJob() error = nil, want server error")
	}
	if !strings.Contains(err.Error(), "403 Forbidden") {
		t.Fatalf("CreateJob() error = %v, want status text", err)
	}
}

func TestClientCreateJobRequiresNamespace(t *testing.T) {
	restClient := &client{
		baseURL:    &url.URL{Scheme: "https", Host: "127.0.0.1"},
		httpClient: &http.Client{},
		userAgent:  "sandbox-runner-test",
	}

	_, err := restClient.CreateJob(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-job"},
	})
	if err == nil {
		t.Fatal("CreateJob() error = nil, want namespace validation error")
	}
	if !strings.Contains(err.Error(), "namespace is required") {
		t.Fatalf("CreateJob() error = %v, want namespace validation", err)
	}
}
