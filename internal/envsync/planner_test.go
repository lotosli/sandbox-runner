package envsync

import "testing"

func TestDefaultStepsUsesWorkspaceRelativePythonVenvPip(t *testing.T) {
	steps := defaultSteps("python", "/tmp/workspace")
	if len(steps) != 2 {
		t.Fatalf("len(steps) = %d, want 2", len(steps))
	}
	if got := steps[1].Cmd[0]; got != ".venv/bin/pip" {
		t.Fatalf("pip step command = %q, want .venv/bin/pip", got)
	}
}
