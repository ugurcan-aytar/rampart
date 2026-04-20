// Package app is the composition root. main.go stays thin; everything about
// wiring (storage, trust, api, subcommands) happens here so it can be tested.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/config"
	"github.com/ugurcan-aytar/rampart/engine/internal/ingestion/sbom/npm"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
	"github.com/ugurcan-aytar/rampart/engine/internal/trust"
)

// App is the engine's runtime. Construct with New, drive with Run, release with Close.
type App struct {
	cfg     *config.Config
	log     *slog.Logger
	storage storage.Storage
	trust   trust.Engine
	server  *http.Server
}

// Main is the top-level dispatcher: subcommand if args[0] matches one, otherwise
// run the server. Keep main.go literally unable to skip this.
func Main(ctx context.Context, args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "parse-sbom":
			return runParseSBOM(ctx, args[1:])
		}
	}
	return runServer(ctx, args)
}

func runServer(ctx context.Context, _ []string) error {
	cfg := config.Default()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	a, err := New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("new app: %w", err)
	}
	defer a.Close()
	return a.Run(ctx)
}

// New wires storage, trust, and the HTTP server.
func New(_ context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	if log == nil {
		log = slog.Default()
	}
	store := memory.New()
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.NewServer(store).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &App{
		cfg:     cfg,
		log:     log,
		storage: store,
		trust:   trust.AlwaysTrust{},
		server:  srv,
	}, nil
}

// Run starts the HTTP server and blocks until ctx is cancelled.
func (a *App) Run(ctx context.Context) error {
	a.log.Info("engine starting", "addr", a.cfg.HTTPAddr)

	errCh := make(chan error, 1)
	go func() {
		if err := a.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		a.log.Info("engine shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return a.server.Shutdown(shutdownCtx)
	}
}

// Close releases storage handles and any other stateful resources.
func (a *App) Close() error {
	return a.storage.Close()
}

// runParseSBOM reads a lockfile from disk, parses it with the Go npm parser,
// and writes the resulting SBOM as indented JSON to stdout.
// Invoked via `engine parse-sbom <path>`.
func runParseSBOM(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("parse-sbom: missing lockfile path")
	}
	path := args[0]
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	sbom, err := npm.NewParser().Parse(ctx, content, npm.LockfileMeta{SourcePath: path})
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(sbom)
}
