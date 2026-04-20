package commands

import (
	"context"
	"errors"
)

// Serve runs the engine daemon. Phase 1 continuation — for now, run
// `go run ./engine/cmd/engine` (or the engine container) directly.
func Serve(_ context.Context, _ []string) error {
	return errors.New("serve: CLI-embedded daemon not wired — use `go run ./engine/cmd/engine` or the engine container directly")
}
