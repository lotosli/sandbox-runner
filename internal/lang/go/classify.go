package golang

import "github.com/lotosli/sandbox-runner/internal/proc"

func Classify(cmd []string) string {
	return proc.ClassifyCommand(cmd)
}
