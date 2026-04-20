// Copyright (c) 2026 Uğurcan Aytar. MIT License.
//
// rampart is the CLI facade over the engine. Subcommand logic lives under
// internal/commands — main.go is just signal plumbing + dispatch.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ugurcan-aytar/rampart/cli/internal/commands"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := commands.Dispatch(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "rampart:", err)
		os.Exit(1)
	}
}
