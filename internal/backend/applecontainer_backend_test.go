package backend

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppleContainerBackendDeleteUsesForce(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "container.log")
	scriptPath := filepath.Join(dir, "container")
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "${APPLE_CONTAINER_TEST_LOG}"
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("APPLE_CONTAINER_TEST_LOG", logPath)

	backend := &AppleContainerBackend{
		binary:    scriptPath,
		sandboxes: map[string]appleContainerRecord{},
		execs:     map[string]*localExecSession{},
	}
	if err := backend.Delete(context.Background(), "ac-test-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "delete --force ac-test-1") {
		t.Fatalf("delete args = %q, want delete --force ac-test-1", string(data))
	}
}
