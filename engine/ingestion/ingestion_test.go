package ingestion_test

import (
	"testing"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/ingestion"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

func TestIngest_StampsIDAndTime(t *testing.T) {
	parsed := &domain.ParsedSBOM{
		Ecosystem:    "npm",
		SourceFormat: "npm-package-lock-v3",
		SourceBytes:  765,
		Packages: []domain.PackageVersion{
			{Ecosystem: "npm", Name: "axios", Version: "1.11.0", PURL: "pkg:npm/axios@1.11.0"},
		},
	}
	before := time.Now().UTC()
	got := ingestion.Ingest(parsed, "kind:Component/default/web", "abc123")
	after := time.Now().UTC()

	// ULID is non-empty and parseable — we don't pin the value (it's
	// time-derived + random), we pin the *shape* so callers can trust
	// sort-by-ID == sort-by-time.
	_, err := ulid.Parse(got.ID)
	require.NoError(t, err, "Ingest must produce a valid ULID")

	// Timestamp falls in the [before, after] window.
	require.False(t, got.GeneratedAt.Before(before), "GeneratedAt earlier than call")
	require.False(t, got.GeneratedAt.After(after), "GeneratedAt later than call")

	// Identity flows through.
	require.Equal(t, "kind:Component/default/web", got.ComponentRef)
	require.Equal(t, "abc123", got.CommitSHA)

	// Parser-pure fields preserved.
	require.Equal(t, parsed.Ecosystem, got.Ecosystem)
	require.Equal(t, parsed.SourceFormat, got.SourceFormat)
	require.Equal(t, parsed.SourceBytes, got.SourceBytes)
	require.Equal(t, parsed.Packages, got.Packages)
}

func TestIngest_EmptyIdentityStillProducesID(t *testing.T) {
	// Degenerate but legal: no component ref, no commit sha. The
	// ingestion layer still stamps ID + GeneratedAt — the SBOM is a
	// first-class domain object regardless of where it came from.
	got := ingestion.Ingest(&domain.ParsedSBOM{Ecosystem: "npm"}, "", "")
	require.NotEmpty(t, got.ID)
	require.False(t, got.GeneratedAt.IsZero())
	require.Empty(t, got.ComponentRef)
	require.Empty(t, got.CommitSHA)
}

func TestIngest_IDsAreMonotonicWithinMillisecond(t *testing.T) {
	// ulid.Make is monotonic (the library docs guarantee non-decreasing
	// within the same millisecond), so two consecutive Ingest calls
	// produce lexicographically-ordered IDs. The incident UI relies on
	// this to list events time-ordered by ID alone.
	a := ingestion.Ingest(&domain.ParsedSBOM{Ecosystem: "npm"}, "", "")
	b := ingestion.Ingest(&domain.ParsedSBOM{Ecosystem: "npm"}, "", "")
	require.Less(t, a.ID, b.ID, "IDs must be non-decreasing across calls")
}
