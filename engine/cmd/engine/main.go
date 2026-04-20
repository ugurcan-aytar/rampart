// Copyright (c) 2026 Uğurcan Aytar. MIT License.
//
// main is intentionally thin — every wiring decision lives in internal/app.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ugurcan-aytar/rampart/engine/internal/app"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := app.Main(ctx, os.Args[1:]); err != nil {
		slog.Error("engine exited with error", "err", err)
		os.Exit(1)
	}
}
