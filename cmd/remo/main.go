package main

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gleicon/remo/cmd/remo/root"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	cmd := root.NewRootCommand(ctx)
	cmd.SetArgs(normalizeArgs(os.Args[1:]))
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func normalizeArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	result := make([]string, len(args))
	copy(result, args)
	for i, arg := range result {
		if len(arg) < 2 || arg[0] != '-' {
			continue
		}
		if strings.HasPrefix(arg, "--") {
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 2 {
			result[i] = "-" + arg
		}
	}
	return result
}
