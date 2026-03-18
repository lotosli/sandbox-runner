package main

import (
	"context"
	"fmt"
	"os"

	"github.com/lotosli/sandbox-runner/internal/cli"
)

func main() {
	code := run(context.Background(), os.Args[1:])
	os.Exit(code)
}

func run(ctx context.Context, args []string) int {
	app, err := cli.Parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	code, err := app.Run(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if code != 0 {
			return code
		}
		return 1
	}
	return code
}
