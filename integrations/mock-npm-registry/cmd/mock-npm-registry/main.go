// Command mock-npm-registry is the demo-only npm registry double.
//
// What it serves:
//
//   - GET /healthz                          → "ok"
//   - GET /-/lockfile/:component            → canned package-lock.json
//     for the demo component. Maps `web-app` and `billing` to
//     axios-compromise.json (malicious axios@1.11.0); `reporting`
//     maps to simple-webapp.json (clean).
//   - GET /-/lockfile/shai-hulud            → a synthetic lockfile
//     with 10 worm-style compromised packages, produced at boot
//     from embedded fixture data.
//   - GET /-/iocs                            → returns the canned
//     IoC list each scenario publishes: axios@1.11.0, the shai-hulud
//     batch, and a vercel-oauth publisherAnomaly entry.
//
// Pure stdlib, no external deps. Serves on :8081 by default; override
// via -addr or MOCK_NPM_ADDR. Intended for Docker Compose only — never
// ship or open to the internet.
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed fixtures/*.json
var fixtures embed.FS

// componentMap routes a demo component to its canned lockfile fixture.
// web-app + billing carry the malicious axios@1.11.0 install; reporting
// stays clean.
var componentMap = map[string]string{
	"web-app":   "axios-compromise.json",
	"billing":   "axios-compromise.json",
	"reporting": "simple-webapp.json",
}

func main() {
	addr := flag.String("addr", envOr("MOCK_NPM_ADDR", ":8081"), "listen address")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("GET /-/lockfile/{component}", func(w http.ResponseWriter, r *http.Request) {
		comp := r.PathValue("component")
		fixture, ok := componentMap[comp]
		if !ok {
			// Synthetic fixtures (shai-hulud, vercel-oauth) are served
			// under the raw component name.
			fixture = comp + ".json"
		}
		body, err := fixtures.ReadFile(filepath.Join("fixtures", fixture))
		if err != nil {
			logger.Warn("unknown component", "component", comp, "err", err)
			http.Error(w, "unknown component "+comp, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("GET /-/iocs", func(w http.ResponseWriter, _ *http.Request) {
		body, err := fixtures.ReadFile("fixtures/iocs.json")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"name":        "rampart-mock-npm-registry",
			"description": "demo-only. See integrations/mock-npm-registry/README.md.",
			"routes":      []string{"/healthz", "/-/lockfile/{component}", "/-/iocs"},
		})
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           logged(logger, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	logger.Info("mock-npm-registry listening", "addr", *addr)

	// Graceful shutdown on SIGINT / SIGTERM so `docker compose down`
	// stops us cleanly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("listen failed", "err", err)
			os.Exit(1)
		}
	}()
	<-sigCh
	logger.Info("shutting down")
	_ = srv.Close()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func logged(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		// Skip healthcheck noise — it fires every 2 seconds and would
		// drown the real traffic.
		if !strings.HasPrefix(r.URL.Path, "/healthz") {
			log.Info("req", "method", r.Method, "path", r.URL.Path,
				"status", rw.status, "dur_ms", time.Since(start).Milliseconds())
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// silence unused import warnings while stdlib-only
var _ = fmt.Sprintf
