package policy

import "os"

func init() {
	getEnv = os.Getenv
}

func max(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
