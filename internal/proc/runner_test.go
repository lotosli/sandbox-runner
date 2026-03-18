package proc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lotosli/sandbox-runner/internal/model"
)

func TestRunnerResolvesCommandFromChildPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX path resolution test")
	}
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, ".sample-bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	scriptPath := filepath.Join(binDir, "child-path-tool")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nprintf 'child-path-ok\\n'\n"), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var lines []string
	result, err := NewRunner().Run(context.Background(), CommandSpec{
		Phase:           model.PhaseExecute,
		Command:         []string{"child-path-tool"},
		Env:             map[string]string{"PATH": ".sample-bin:$PATH"},
		Dir:             workspace,
		Timeout:         5 * time.Second,
		RunID:           "r-path",
		Attempt:         1,
		CommandClass:    "test.run",
		ArtifactDir:     filepath.Join(workspace, ".sandbox-runner"),
		LogLineMaxBytes: 1024,
	}, testIOHandler(func(log model.StructuredLog) {
		lines = append(lines, log.Line)
	}))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if len(lines) != 1 || lines[0] != "child-path-ok" {
		t.Fatalf("stdout lines = %#v, want [child-path-ok]", lines)
	}
}

type testIOHandler func(model.StructuredLog)

func (f testIOHandler) OnLog(_ context.Context, log model.StructuredLog) error {
	f(log)
	return nil
}
