package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type proof struct {
	Language  string `json:"language"`
	Phase     string `json:"phase"`
	RunID     string `json:"run_id,omitempty"`
	Attempt   string `json:"attempt,omitempty"`
	SandboxID string `json:"sandbox_id,omitempty"`
}

func main() {
	phase := "execute"
	if len(os.Args) > 1 {
		phase = os.Args[1]
	}

	proofPath := filepath.Join(".sandbox-runner", "artifacts", "proof.json")
	if phase == "verify" {
		if _, err := os.Stat(proofPath); err != nil {
			fmt.Fprintln(os.Stderr, "missing proof artifact")
			os.Exit(1)
		}
	}

	if err := os.MkdirAll(filepath.Dir(proofPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	file, err := os.Create(proofPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer file.Close()

	if err := json.NewEncoder(file).Encode(proof{
		Language:  "go",
		Phase:     phase,
		RunID:     os.Getenv("RUN_ID"),
		Attempt:   os.Getenv("ATTEMPT"),
		SandboxID: os.Getenv("SANDBOX_ID"),
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	switch phase {
	case "execute":
		fmt.Println("__GO_EXECUTE__")
		fmt.Printf("RUN_ID=%s\n", os.Getenv("RUN_ID"))
		fmt.Fprintf(os.Stderr, "__GO_STDERR__\n")
	case "verify":
		fmt.Println("__GO_VERIFY__")
		fmt.Printf("SANDBOX_ID=%s\n", os.Getenv("SANDBOX_ID"))
	default:
		fmt.Fprintf(os.Stderr, "unsupported phase %q\n", phase)
		os.Exit(2)
	}
}
