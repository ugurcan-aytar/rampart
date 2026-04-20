package commands

import (
	"context"
	"errors"
	"io"
)

// Ingest submits an SBOM or an IoC to a running engine over HTTP.
// Phase 1 continuation — requires the publishing endpoints to come online.
func Ingest(_ context.Context, _ []string, _ io.Writer) error {
	return errors.New("ingest: not yet implemented — POST /v1/iocs and /v1/components/{ref}/sboms land in Phase 1 continuation")
}
