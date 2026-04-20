// Copyright (c) 2026 Uğurcan Aytar. MIT License.
//
// slack-notifier subscribes to the rampart engine's /v1/stream SSE feed
// and posts a Slack message whenever an `incident.opened` event arrives.
//
// Architecture note: this binary's go.mod does NOT import the engine's
// Go packages. It speaks HTTP/SSE only — which makes it the first concrete
// example of rampart's adapter pattern: the engine owns the contract
// (schemas/openapi.yaml + text/event-stream), consumers attach by reading
// the wire, not by linking to internals.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ugurcan-aytar/rampart/integrations/slack-notifier/internal/receiver"
	"github.com/ugurcan-aytar/rampart/integrations/slack-notifier/internal/webhook"
)

func main() {
	engineURL := flag.String("engine-url", envOr("RAMPART_ENGINE_URL", "http://localhost:8080"), "engine base URL")
	webhookURL := flag.String("webhook", os.Getenv("SLACK_WEBHOOK_URL"), "Slack incoming webhook (env: SLACK_WEBHOOK_URL)")
	dryRun := flag.Bool("dry-run", false, "log would-be Slack payloads instead of sending")
	flag.Parse()

	// If no webhook configured, flip on dry-run so the binary is still
	// useful for local testing.
	if *webhookURL == "" && !*dryRun {
		*dryRun = true
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	var sender webhook.Sender
	if *dryRun {
		sender = webhook.NewDryRun(logger)
		logger.Info("dry-run mode — Slack payloads will be logged, not sent")
	} else {
		sender = webhook.NewHTTP(*webhookURL, logger)
	}

	streamURL := *engineURL + "/v1/stream"
	logger.Info("subscribing to engine SSE", "url", streamURL)

	client := receiver.New(streamURL, logger)
	err := client.Run(ctx, func(f receiver.Frame) {
		if f.EventType != "incident.opened" {
			logger.Debug("ignoring non-incident event", "type", f.EventType)
			return
		}
		if err := sender.Send(ctx, f); err != nil {
			logger.Error("slack send failed", "err", err, "incident_id", f.ID)
		}
	})
	if err != nil && err != context.Canceled {
		fmt.Fprintln(os.Stderr, "slack-notifier:", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
