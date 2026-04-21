// Package storagetest holds the shared contract suite Storage backends run
// against. Each concrete backend (memory, sqlite, postgres, …) dispatches
// here from its *_test.go so a single behaviour spec covers every backend.
package storagetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// Run executes the contract suite. newStore must return a fresh, empty store
// on every call — the suite does not clean up between sub-tests.
func Run(t *testing.T, newStore func() storage.Storage) {
	t.Helper()
	t.Run("Component", func(t *testing.T) { testComponent(t, newStore) })
	t.Run("SBOM", func(t *testing.T) { testSBOM(t, newStore) })
	t.Run("IoC", func(t *testing.T) { testIoC(t, newStore) })
	t.Run("Incident", func(t *testing.T) { testIncident(t, newStore) })
	t.Run("Remediation", func(t *testing.T) { testRemediation(t, newStore) })
	t.Run("Publisher", func(t *testing.T) { testPublisher(t, newStore) })
}

func testComponent(t *testing.T, newStore func() storage.Storage) {
	t.Helper()
	ctx := context.Background()
	s := newStore()
	defer s.Close()

	c := domain.Component{
		Ref: "kind:Component/default/web-app", Kind: "Component",
		Namespace: "default", Name: "web-app", Owner: "team:platform",
	}
	require.NoError(t, s.UpsertComponent(ctx, c))

	got, err := s.GetComponent(ctx, c.Ref)
	require.NoError(t, err)
	require.Equal(t, c.Ref, got.Ref)

	c.Owner = "team:security"
	require.NoError(t, s.UpsertComponent(ctx, c), "upsert must be idempotent")
	got, err = s.GetComponent(ctx, c.Ref)
	require.NoError(t, err)
	require.Equal(t, "team:security", got.Owner, "upsert must overwrite")

	_, err = s.GetComponent(ctx, "kind:Component/default/missing")
	require.True(t, errors.Is(err, storage.ErrNotFound))

	require.NoError(t, s.UpsertComponent(ctx, domain.Component{Ref: "kind:Component/default/a"}))
	require.NoError(t, s.UpsertComponent(ctx, domain.Component{Ref: "kind:Component/default/z"}))
	list, err := s.ListComponents(ctx)
	require.NoError(t, err)
	require.Equal(t, 3, len(list))
	require.Equal(t, "kind:Component/default/a", list[0].Ref, "ListComponents must sort by ref")
	require.Equal(t, "kind:Component/default/z", list[2].Ref)
}

func testSBOM(t *testing.T, newStore func() storage.Storage) {
	t.Helper()
	ctx := context.Background()
	s := newStore()
	defer s.Close()

	now := time.Now().UTC()
	ref := "kind:Component/default/web-app"
	s1 := domain.SBOM{ID: "sbom-1", ComponentRef: ref, Ecosystem: "npm", GeneratedAt: now.Add(-2 * time.Hour)}
	s2 := domain.SBOM{ID: "sbom-2", ComponentRef: ref, Ecosystem: "npm", GeneratedAt: now.Add(-1 * time.Hour)}
	other := domain.SBOM{ID: "sbom-3", ComponentRef: "kind:Component/default/api", Ecosystem: "npm", GeneratedAt: now}

	require.NoError(t, s.UpsertSBOM(ctx, s1))
	require.NoError(t, s.UpsertSBOM(ctx, s2))
	require.NoError(t, s.UpsertSBOM(ctx, other))

	got, err := s.GetSBOM(ctx, "sbom-1")
	require.NoError(t, err)
	require.Equal(t, ref, got.ComponentRef)

	byComp, err := s.ListSBOMsByComponent(ctx, ref)
	require.NoError(t, err)
	require.Equal(t, 2, len(byComp))
	require.Equal(t, "sbom-1", byComp[0].ID, "oldest first")
	require.Equal(t, "sbom-2", byComp[1].ID)

	_, err = s.GetSBOM(ctx, "missing")
	require.True(t, errors.Is(err, storage.ErrNotFound))
}

func testIoC(t *testing.T, newStore func() storage.Storage) {
	t.Helper()
	ctx := context.Background()
	s := newStore()
	defer s.Close()

	i := domain.IoC{
		ID: "ioc-1", Kind: domain.IoCKindPackageVersion, Severity: domain.SeverityCritical,
		Ecosystem: "npm", PublishedAt: time.Now().UTC(),
		PackageVersion: &domain.IoCPackageVersion{Name: "axios", Version: "1.11.0", PURL: "pkg:npm/axios@1.11.0"},
	}
	require.NoError(t, s.UpsertIoC(ctx, i))

	got, err := s.GetIoC(ctx, i.ID)
	require.NoError(t, err)
	require.Equal(t, domain.SeverityCritical, got.Severity)
	require.NotNil(t, got.PackageVersion)
	require.Equal(t, "axios", got.PackageVersion.Name)

	list, err := s.ListIoCs(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, len(list))

	_, err = s.GetIoC(ctx, "missing")
	require.True(t, errors.Is(err, storage.ErrNotFound))
}

func testIncident(t *testing.T, newStore func() storage.Storage) {
	t.Helper()
	ctx := context.Background()
	s := newStore()
	defer s.Close()

	now := time.Now().UTC()
	inc := domain.Incident{
		ID: "inc-1", IoCID: "ioc-1", State: domain.StatePending,
		OpenedAt: now, LastTransitionedAt: now,
		AffectedComponentsSnapshot: []string{"kind:Component/default/web"},
	}
	require.NoError(t, s.UpsertIncident(ctx, inc))

	got, err := s.GetIncident(ctx, inc.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StatePending, got.State)

	require.NoError(t, got.Transition(domain.StateTriaged, now.Add(time.Minute)))
	require.NoError(t, s.UpsertIncident(ctx, *got))
	got, err = s.GetIncident(ctx, inc.ID)
	require.NoError(t, err)
	require.Equal(t, domain.StateTriaged, got.State)
	require.Equal(t,
		[]string{"kind:Component/default/web"},
		got.AffectedComponentsSnapshot,
		"snapshot must remain frozen after state transition")

	_, err = s.GetIncident(ctx, "missing")
	require.True(t, errors.Is(err, storage.ErrNotFound))
}

func testRemediation(t *testing.T, newStore func() storage.Storage) {
	t.Helper()
	ctx := context.Background()
	s := newStore()
	defer s.Close()

	now := time.Now().UTC()
	inc := domain.Incident{
		ID: "inc-2", IoCID: "ioc-2", State: domain.StatePending,
		OpenedAt: now, LastTransitionedAt: now,
	}
	require.NoError(t, s.UpsertIncident(ctx, inc))

	r1 := domain.Remediation{
		ID: "r1", IncidentID: inc.ID,
		Kind: domain.RemediationNotify, ExecutedAt: now,
		ActorRef: "user:alice",
	}
	require.NoError(t, s.AppendRemediation(ctx, inc.ID, r1))

	r2 := domain.Remediation{
		ID: "r2", IncidentID: inc.ID,
		Kind: domain.RemediationPinVersion, ExecutedAt: now.Add(time.Minute),
		ActorRef: "user:bob",
	}
	require.NoError(t, s.AppendRemediation(ctx, inc.ID, r2))

	rs, err := s.ListRemediations(ctx, inc.ID)
	require.NoError(t, err)
	require.Len(t, rs, 2)
	require.Equal(t, "r1", rs[0].ID, "append-only: order must match write order")
	require.Equal(t, "r2", rs[1].ID)

	// Remediations must also appear on the stored incident — that's how
	// GetIncident hydrates them in one round-trip.
	got, err := s.GetIncident(ctx, inc.ID)
	require.NoError(t, err)
	require.Len(t, got.Remediations, 2)

	_, err = s.ListRemediations(ctx, "missing")
	require.True(t, errors.Is(err, storage.ErrNotFound))
	err = s.AppendRemediation(ctx, "missing", r1)
	require.True(t, errors.Is(err, storage.ErrNotFound))
}

func testPublisher(t *testing.T, newStore func() storage.Storage) {
	t.Helper()
	ctx := context.Background()
	s := newStore()
	defer s.Close()

	now := time.Now().UTC()
	p := domain.Publisher{
		Ecosystem: "npm", Name: "axios-maintainer",
		FirstSeen: now.Add(-365 * 24 * time.Hour), LastSeen: now,
	}
	require.NoError(t, s.UpsertPublisher(ctx, p))

	got, err := s.GetPublisher(ctx, "npm", "axios-maintainer")
	require.NoError(t, err)
	require.Equal(t, p.Ecosystem, got.Ecosystem)
	require.Equal(t, p.Name, got.Name)

	profile := domain.PublisherProfile{
		Publisher: p, PackageCount: 5, PublishCount: 100, Last30DayPublishes: 2,
		UsesOIDC: false, HasGitTags: true,
		MaintainerEmails: []string{"legit@example.com"},
	}
	require.NoError(t, s.UpsertPublisherProfile(ctx, profile))

	gotProfile, err := s.GetPublisherProfile(ctx, "npm", "axios-maintainer")
	require.NoError(t, err)
	require.Equal(t, 5, gotProfile.PackageCount)
	require.Equal(t, []string{"legit@example.com"}, gotProfile.MaintainerEmails)

	require.NoError(t, s.UpsertPublisher(ctx, domain.Publisher{Ecosystem: "npm", Name: "z-dev"}))
	require.NoError(t, s.UpsertPublisher(ctx, domain.Publisher{Ecosystem: "pypi", Name: "py-dev"}))

	npmList, err := s.ListPublishers(ctx, "npm")
	require.NoError(t, err)
	require.Equal(t, 2, len(npmList), "ListPublishers must filter by ecosystem")

	pypiList, err := s.ListPublishers(ctx, "pypi")
	require.NoError(t, err)
	require.Equal(t, 1, len(pypiList))

	_, err = s.GetPublisher(ctx, "npm", "missing")
	require.True(t, errors.Is(err, storage.ErrNotFound))

	_, err = s.GetPublisherProfile(ctx, "npm", "missing")
	require.True(t, errors.Is(err, storage.ErrNotFound))
}
