package proc

import (
	"path/filepath"
	"strings"

	"github.com/google/shlex"
)

func ClassifyCommand(cmd []string) string {
	tokens := normalizeCommand(cmd)
	if len(tokens) == 0 {
		return "command.exec"
	}
	head := filepath.Base(tokens[0])
	switch head {
	case "pytest":
		return "test.run"
	case "go":
		if len(tokens) >= 2 {
			switch tokens[1] {
			case "test":
				if contains(tokens, "-bench") {
					return "benchmark.run"
				}
				return "test.run"
			case "run":
				return "app.start"
			case "build":
				return "build.run"
			case "generate":
				return "codegen.run"
			}
		}
	case "npm", "pnpm":
		if len(tokens) >= 2 {
			switch tokens[1] {
			case "test":
				return "test.run"
			case "build":
				return "build.run"
			}
		}
	case "mvn":
		if contains(tokens, "package") || contains(tokens, "verify") {
			return "build.run"
		}
	case "git":
		if len(tokens) >= 2 && tokens[1] == "apply" {
			return "patch.apply"
		}
	case "patch":
		return "patch.apply"
	case "python", "python3":
		if containsSubstring(tokens, "smoke") {
			return "smoke.run"
		}
		return "app.start"
	case "node":
		return "app.start"
	case "java":
		return "app.start"
	case "curl":
		if containsSubstring(tokens, "health") || containsSubstring(tokens, "ready") {
			return "smoke.run"
		}
	case "dlv":
		return "debug.run"
	}

	if strings.HasPrefix(head, ".") || strings.HasPrefix(head, "/") {
		return "app.start"
	}
	return "command.exec"
}

func ToolName(cmd []string) string {
	tokens := normalizeCommand(cmd)
	if len(tokens) == 0 {
		return ""
	}
	return filepath.Base(tokens[0])
}

func normalizeCommand(cmd []string) []string {
	if len(cmd) == 0 {
		return nil
	}
	if len(cmd) >= 3 && (cmd[0] == "bash" || cmd[0] == "sh") && (cmd[1] == "-lc" || cmd[1] == "-c") {
		if parsed, err := shlex.Split(cmd[2]); err == nil && len(parsed) > 0 {
			return parsed
		}
	}
	return cmd
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func containsSubstring(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}
