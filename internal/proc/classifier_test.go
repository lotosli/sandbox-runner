package proc

import "testing"

func TestClassifyCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  []string
		want string
	}{
		{name: "go test", cmd: []string{"go", "test", "./..."}, want: "test.run"},
		{name: "go bench", cmd: []string{"go", "test", "-bench", "."}, want: "benchmark.run"},
		{name: "go build", cmd: []string{"go", "build", "./..."}, want: "build.run"},
		{name: "bash inner command", cmd: []string{"bash", "-lc", "go test ./..."}, want: "test.run"},
		{name: "patch", cmd: []string{"git", "apply", "x.patch"}, want: "patch.apply"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyCommand(tt.cmd); got != tt.want {
				t.Fatalf("ClassifyCommand(%v) = %s, want %s", tt.cmd, got, tt.want)
			}
		})
	}
}
