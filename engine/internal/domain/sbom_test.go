package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

func TestSBOM_Fields(t *testing.T) {
	now := time.Now().UTC()
	s := domain.SBOM{
		ID:           "01JKX",
		ComponentRef: "kind:Component/default/web",
		CommitSHA:    "abc123",
		Ecosystem:    "npm",
		GeneratedAt:  now,
		SourceFormat: "npm-package-lock-v3",
		SourceBytes:  4096,
		Packages: []domain.PackageVersion{
			{Ecosystem: "npm", Name: "axios", Version: "1.11.0", PURL: "pkg:npm/axios@1.11.0"},
		},
	}
	require.Equal(t, "01JKX", s.ID)
	require.Equal(t, "npm", s.Ecosystem)
	require.Equal(t, 1, len(s.Packages))
	require.Equal(t, "npm-package-lock-v3", s.SourceFormat)
	require.Equal(t, int64(4096), s.SourceBytes)
	require.WithinDuration(t, now, s.GeneratedAt, 0)
}
