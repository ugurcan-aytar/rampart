package commands

import (
	"context"
	"errors"
	"io"
)

// Status fetches one incident by id from a running engine.
// Phase 1 continuation — needs /v1/incidents/{id} wired first.
func Status(_ context.Context, _ []string, _ io.Writer) error {
	return errors.New("status: not yet implemented — GET /v1/incidents/{id} is a 501 stub in Adım 3")
}
