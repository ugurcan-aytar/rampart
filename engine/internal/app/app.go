// Package app is the composition root. main.go stays thin; everything about
// wiring (storage, trust, api, subcommands) happens here so it can be tested.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/ingestion"
	"github.com/ugurcan-aytar/rampart/engine/internal/api"
	"github.com/ugurcan-aytar/rampart/engine/internal/api/middleware"
	"github.com/ugurcan-aytar/rampart/engine/internal/config"
	"github.com/ugurcan-aytar/rampart/engine/internal/events"
	"github.com/ugurcan-aytar/rampart/engine/internal/ingestion/native"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage/memory"
	pgstorage "github.com/ugurcan-aytar/rampart/engine/internal/storage/postgres"
	"github.com/ugurcan-aytar/rampart/engine/internal/trust"
	"github.com/ugurcan-aytar/rampart/engine/sbom/npm"
)

// defaultNativeSocket resolves RAMPART_NATIVE_SOCKET or falls back to
// the Unix /tmp path rampart-native uses by default.
func defaultNativeSocket() string {
	if s := os.Getenv("RAMPART_NATIVE_SOCKET"); s != "" {
		return s
	}
	return "/tmp/rampart-native.sock"
}

// App is the engine's runtime. Construct with New, drive with Run, release with Close.
type App struct {
	cfg               *config.Config
	log               *slog.Logger
	storage           storage.Storage
	trust             trust.Engine
	events            *events.Bus
	server            *http.Server
	listener          net.Listener
	effectiveStrategy npm.Strategy
}

// Addr reports the host:port the server actually bound to. When the
// caller configures `:0` (ephemeral), this returns the resolved port
// — useful to integration tests that need to issue HTTP calls without
// racing against the OS listen. Valid only after Run has begun; the
// nil receiver and nil listener both return the configured address.
func (a *App) Addr() string {
	if a == nil {
		return ""
	}
	if a.listener != nil {
		return a.listener.Addr().String()
	}
	return a.cfg.HTTPAddr
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
	cfg := config.FromEnv()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	a, err := New(ctx, cfg, logger)
	if err != nil {
		return fmt.Errorf("new app: %w", err)
	}
	defer a.Close()
	return a.Run(ctx)
}

// New wires storage, trust, the event bus, and the HTTP server.
//
// The parser strategy is resolved here so the engine logs what it's
// actually going to use on the first ingestion request, not just what
// the operator asked for: requested=native with the sidecar down
// yields `effective=go` and a warn entry, per ADR-0005 Final Decision.
func New(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	if log == nil {
		log = slog.Default()
	}
	store, err := openStorage(ctx, cfg, log)
	if err != nil {
		return nil, err
	}
	bus := events.NewBus(cfg.SSESubscriberBuffer)

	requested := npm.Strategy(cfg.ParserStrategy)
	nativeClient := native.New(cfg.NativeSocketPath)
	effective := npm.EffectiveStrategy(ctx, requested, nativeClient, log)
	log.Info("parser strategy resolved",
		"requested", string(requested),
		"effective", string(effective),
		"native_socket", cfg.NativeSocketPath)

	apiServer := api.NewServer(store, bus, cfg.SSEHeartbeatInterval)
	apiServer.SetAuth(middleware.AuthOptions{
		Enabled:     cfg.AuthEnabled,
		SigningKey:  cfg.AuthSigningKey,
		Algorithm:   cfg.AuthAlgorithm,
		Audience:    cfg.AuthAudience,
		ExemptPaths: middleware.DefaultExemptPaths,
	})
	corsOpts := middleware.DefaultCORSOptions()
	corsOpts.AllowAll = cfg.CORSAllowAll
	corsOpts.Origins = cfg.CORSOrigins
	apiServer.SetCORS(corsOpts)
	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return &App{
		cfg:               cfg,
		log:               log,
		storage:           store,
		trust:             trust.AlwaysTrust{},
		events:            bus,
		server:            srv,
		effectiveStrategy: effective,
	}, nil
}

// openStorage wires the configured storage backend. `memory` is a
// no-dependency in-process map used by tests and throwaway demos;
// `postgres` is the production default — it runs goose migrations
// before returning the pool, so a fresh database works end-to-end
// after one boot. Missing DSN on `postgres` is a fail-fast error.
func openStorage(ctx context.Context, cfg *config.Config, log *slog.Logger) (storage.Storage, error) {
	switch cfg.StorageBackend {
	case "", "memory":
		log.Info("storage backend: memory")
		return memory.New(), nil
	case "postgres":
		if cfg.DBDSN == "" {
			return nil, errors.New("storage=postgres but RAMPART_DB_DSN is empty")
		}
		if err := pgstorage.MigrateUp(ctx, cfg.DBDSN); err != nil {
			return nil, fmt.Errorf("postgres: migrate: %w", err)
		}
		s, err := pgstorage.Open(ctx, cfg.DBDSN, cfg.DBMaxConns)
		if err != nil {
			return nil, err
		}
		log.Info("storage backend: postgres", "max_conns", cfg.DBMaxConns)
		return s, nil
	default:
		return nil, fmt.Errorf("unknown RAMPART_STORAGE=%q (expected memory or postgres)", cfg.StorageBackend)
	}
}

// Run starts the HTTP server and blocks until ctx is cancelled.
// Run binds the listener synchronously before returning from the
// goroutine spawn so callers that inspect Addr immediately after Run
// see the resolved port — matters when cfg.HTTPAddr asks for an
// ephemeral port (`:0`) in integration tests.
func (a *App) Run(ctx context.Context) error {
	l, err := net.Listen("tcp", a.cfg.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", a.cfg.HTTPAddr, err)
	}
	a.listener = l
	a.log.Info("engine starting", "addr", l.Addr().String())

	errCh := make(chan error, 1)
	go func() {
		if err := a.server.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
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

// runParseSBOM reads a lockfile from disk, parses it with the selected
// npm parser backend, and writes the resulting SBOM as indented JSON to
// stdout.
//
// Invoked via:
//
//	engine parse-sbom
//	    [--parser go|native]
//	    [--component-ref ref]
//	    [--commit-sha sha]
//	    [--native-socket /path]
//	    <lockfile>
//
// With neither `--component-ref` nor `--commit-sha`, the command emits a
// pure ParsedSBOM (no ID, no GeneratedAt). When either is supplied, the
// engine runs the ingestion layer to produce a full SBOM with a freshly
// minted ULID. Mirrors the CLI's behaviour in `cli/internal/commands/scan.go`.
func runParseSBOM(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("parse-sbom", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	parser := fs.String("parser", "go", "parser backend: go | native")
	componentRef := fs.String("component-ref", "", "component reference (e.g. component:default/web-app)")
	commitSHA := fs.String("commit-sha", "", "commit sha the SBOM was taken at")
	nativeSocket := fs.String("native-socket", defaultNativeSocket(), "UDS path for rampart-native (parser=native only)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: rampart parse-sbom [--parser go|native] [--component-ref ref] [--commit-sha sha] [--native-socket path] <lockfile>")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		fs.Usage()
		return errors.New("parse-sbom: missing lockfile path")
	}
	path := fs.Arg(0)

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	strategy := npm.Strategy(*parser)
	strategyParser := npm.NewStrategyParser(
		strategy,
		npm.NewParser(),
		native.New(*nativeSocket),
	)
	fmt.Fprintf(os.Stderr, "parse-sbom: strategy=%s socket=%s bytes=%d\n",
		strategy, *nativeSocket, len(content))

	parsed, err := strategyParser.Parse(ctx, content)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if *componentRef == "" && *commitSHA == "" {
		return enc.Encode(parsed)
	}
	return enc.Encode(ingestion.Ingest(parsed, *componentRef, *commitSHA))
}
