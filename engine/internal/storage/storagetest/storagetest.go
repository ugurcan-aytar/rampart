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
	t.Run("PublisherSnapshot", func(t *testing.T) { testPublisherSnapshot(t, newStore) })
	t.Run("Anomaly", func(t *testing.T) { testAnomaly(t, newStore) })
	t.Run("IncidentFilter", func(t *testing.T) { testIncidentFilter(t, newStore) })
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

func testPublisherSnapshot(t *testing.T, newStore func() storage.Storage) {
	t.Helper()
	ctx := context.Background()
	s := newStore()
	defer s.Close()

	now := time.Now().UTC()
	older := now.Add(-2 * time.Hour)
	older2 := now.Add(-90 * time.Minute)

	pubAt := now.Add(-30 * 24 * time.Hour)
	url := "https://github.com/axios/axios"
	axiosOlder := domain.PublisherSnapshot{
		PackageRef: "npm:axios",
		SnapshotAt: older,
		Maintainers: []domain.Maintainer{
			{Email: "a@example.com", Name: "Maintainer A", Username: "maint-a"},
		},
		LatestVersion:            "1.10.0",
		LatestVersionPublishedAt: &pubAt,
		PublishMethod:            "token",
		SourceRepoURL:            &url,
		RawData:                  []byte(`{"name":"axios"}`),
	}
	axiosNewer := axiosOlder
	axiosNewer.SnapshotAt = now
	axiosNewer.LatestVersion = "1.11.0"
	axiosNewer.PublishMethod = "oidc-trusted-publisher"

	// Save older first so the natural insertion order does NOT match
	// the contract's newest-first read order — forces the backend to
	// actually sort by snapshot_at DESC rather than rely on insertion.
	require.NoError(t, s.SavePublisherSnapshot(ctx, axiosOlder))
	require.NoError(t, s.SavePublisherSnapshot(ctx, axiosNewer))

	hist, err := s.GetPublisherHistory(ctx, "npm:axios", 0)
	require.NoError(t, err)
	require.Len(t, hist, 2, "history must contain both snapshots")
	require.True(t, hist[0].SnapshotAt.After(hist[1].SnapshotAt),
		"GetPublisherHistory must return newest-first")
	require.Equal(t, "1.11.0", hist[0].LatestVersion)
	require.Equal(t, "oidc-trusted-publisher", hist[0].PublishMethod)
	require.Equal(t, "Maintainer A", hist[0].Maintainers[0].Name)
	require.NotNil(t, hist[0].SourceRepoURL)
	require.Equal(t, url, *hist[0].SourceRepoURL)
	require.NotZero(t, hist[0].ID, "ID must be assigned by the store")
	require.NotEqual(t, hist[0].ID, hist[1].ID, "snapshot IDs must be distinct")

	// Limit caps the result without re-ordering.
	limited, err := s.GetPublisherHistory(ctx, "npm:axios", 1)
	require.NoError(t, err)
	require.Len(t, limited, 1)
	require.Equal(t, "1.11.0", limited[0].LatestVersion)

	// Unknown package_ref returns empty, not ErrNotFound.
	miss, err := s.GetPublisherHistory(ctx, "npm:unknown-pkg", 0)
	require.NoError(t, err)
	require.Empty(t, miss)

	// ListPackagesNeedingRefresh — a second package whose only snapshot
	// is between older and now should NOT show up when threshold is
	// older2 (between older and now).
	cargoSnap := domain.PublisherSnapshot{
		PackageRef: "cargo:tokio",
		SnapshotAt: now,
	}
	require.NoError(t, s.SavePublisherSnapshot(ctx, cargoSnap))

	stale, err := s.ListPackagesNeedingRefresh(ctx, older2, 0)
	require.NoError(t, err)
	// axios's MAX(snapshot_at) is `now` (newer than older2 → not stale).
	// Wait — axios's MAX is `now`, cargo's MAX is `now`. Neither is
	// older than older2; expect empty.
	require.Empty(t, stale,
		"no package's latest snapshot is older than the threshold")

	// Bump the threshold past `now` so both packages are stale.
	stale2, err := s.ListPackagesNeedingRefresh(ctx, now.Add(time.Minute), 0)
	require.NoError(t, err)
	require.Len(t, stale2, 2)

	// Limit honoured.
	stale3, err := s.ListPackagesNeedingRefresh(ctx, now.Add(time.Minute), 1)
	require.NoError(t, err)
	require.Len(t, stale3, 1)
}

func testAnomaly(t *testing.T, newStore func() storage.Storage) {
	t.Helper()
	ctx := context.Background()
	s := newStore()
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Microsecond) // postgres truncates μs
	earlier := now.Add(-2 * time.Hour)

	a1 := domain.Anomaly{
		Kind:        domain.AnomalyKindMaintainerEmailDrift,
		PackageRef:  "npm:axios",
		DetectedAt:  now,
		Confidence:  domain.ConfidenceHigh,
		Explanation: "new maintainer email registered 3h before publish",
		Evidence:    map[string]any{"new_email": "x@bad.io", "publish_age_hours": float64(3)},
	}
	a2 := domain.Anomaly{
		Kind:        domain.AnomalyKindOIDCPublishingRegression,
		PackageRef:  "npm:axios",
		DetectedAt:  earlier,
		Confidence:  domain.ConfidenceMedium,
		Explanation: "publish_method went OIDC → token",
		Evidence:    map[string]any{},
	}
	a3 := domain.Anomaly{
		Kind:       domain.AnomalyKindVersionJump,
		PackageRef: "npm:left-pad",
		DetectedAt: now,
		Confidence: domain.ConfidenceLow,
		Evidence:   map[string]any{"old_major": float64(1), "new_major": float64(99)},
	}

	id1, err := s.SaveAnomaly(ctx, a1)
	require.NoError(t, err)
	require.NotZero(t, id1)
	id2, err := s.SaveAnomaly(ctx, a2)
	require.NoError(t, err)
	require.NotEqual(t, id1, id2)
	id3, err := s.SaveAnomaly(ctx, a3)
	require.NoError(t, err)

	// Idempotency: re-saving the same (kind, package_ref, detected_at)
	// returns the existing id, does not duplicate.
	id1again, err := s.SaveAnomaly(ctx, a1)
	require.NoError(t, err)
	require.Equal(t, id1, id1again, "dedup must return existing id")

	// Get by id round-trips fields including Evidence.
	got, err := s.GetAnomaly(ctx, id1)
	require.NoError(t, err)
	require.Equal(t, domain.AnomalyKindMaintainerEmailDrift, got.Kind)
	require.Equal(t, "npm:axios", got.PackageRef)
	require.Equal(t, domain.ConfidenceHigh, got.Confidence)
	require.Equal(t, "x@bad.io", got.Evidence["new_email"])

	_, err = s.GetAnomaly(ctx, 999_999)
	require.True(t, errors.Is(err, storage.ErrNotFound))

	// ListAnomalies — newest first across the table.
	all, err := s.ListAnomalies(ctx, domain.AnomalyFilter{})
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.True(t, !all[0].DetectedAt.Before(all[1].DetectedAt), "newest-first")

	// Filter by package_ref.
	axiosOnly, err := s.ListAnomalies(ctx, domain.AnomalyFilter{PackageRef: "npm:axios"})
	require.NoError(t, err)
	require.Len(t, axiosOnly, 2)
	for _, a := range axiosOnly {
		require.Equal(t, "npm:axios", a.PackageRef)
	}

	// Filter by kind.
	regressionOnly, err := s.ListAnomalies(ctx, domain.AnomalyFilter{
		Kind: domain.AnomalyKindOIDCPublishingRegression,
	})
	require.NoError(t, err)
	require.Len(t, regressionOnly, 1)
	require.Equal(t, id2, regressionOnly[0].ID)

	// Time window: only the `now` anomalies.
	window := now.Add(-time.Minute)
	recent, err := s.ListAnomalies(ctx, domain.AnomalyFilter{From: &window})
	require.NoError(t, err)
	require.Len(t, recent, 2)
	for _, a := range recent {
		require.False(t, a.DetectedAt.Before(window))
	}

	// Limit caps result count.
	limited, err := s.ListAnomalies(ctx, domain.AnomalyFilter{Limit: 1})
	require.NoError(t, err)
	require.Len(t, limited, 1)

	_ = id3 // referenced via the `all` count assertion
}

func testIncidentFilter(t *testing.T, newStore func() storage.Storage) {
	t.Helper()
	ctx := context.Background()
	s := newStore()
	defer s.Close()

	now := time.Now().UTC().Truncate(time.Microsecond)

	// Three components, two owners.
	require.NoError(t, s.UpsertComponent(ctx, domain.Component{
		Ref: "kind:Component/default/billing", Kind: "Component",
		Namespace: "default", Name: "billing", Owner: "team-payments",
	}))
	require.NoError(t, s.UpsertComponent(ctx, domain.Component{
		Ref: "kind:Component/default/web-app", Kind: "Component",
		Namespace: "default", Name: "web-app", Owner: "team-platform",
	}))
	require.NoError(t, s.UpsertComponent(ctx, domain.Component{
		Ref: "kind:Component/default/reporting", Kind: "Component",
		Namespace: "default", Name: "reporting", Owner: "team-data",
	}))

	// Two IoCs in different ecosystems.
	require.NoError(t, s.UpsertIoC(ctx, domain.IoC{
		ID: "ioc-npm", Kind: domain.IoCKindPackageVersion, Severity: domain.SeverityHigh,
		Ecosystem: "npm", Source: "test", PublishedAt: now,
		PackageVersion: &domain.IoCPackageVersion{Name: "axios", Version: "1.11.0"},
	}))
	require.NoError(t, s.UpsertIoC(ctx, domain.IoC{
		ID: "ioc-gomod", Kind: domain.IoCKindPackageVersion, Severity: domain.SeverityCritical,
		Ecosystem: "gomod", Source: "test", PublishedAt: now,
		PackageVersion: &domain.IoCPackageVersion{Name: "github.com/spf13/cobra", Version: "1.8.0"},
	}))

	mkInc := func(id, iocID string, state domain.IncidentState, opened time.Time, snapshot []string) {
		require.NoError(t, s.UpsertIncident(ctx, domain.Incident{
			ID: id, IoCID: iocID, State: state, OpenedAt: opened, LastTransitionedAt: opened,
			AffectedComponentsSnapshot: snapshot,
		}))
	}

	mkInc("inc-A-pending-old", "ioc-npm", domain.StatePending, now.Add(-3*time.Hour),
		[]string{"kind:Component/default/billing"})
	mkInc("inc-B-triaged-mid", "ioc-npm", domain.StateTriaged, now.Add(-90*time.Minute),
		[]string{"kind:Component/default/web-app"})
	mkInc("inc-C-pending-recent", "ioc-gomod", domain.StatePending, now.Add(-30*time.Minute),
		[]string{"kind:Component/default/reporting"})

	// 1) No filter → 3 incidents, newest-first.
	all, err := s.ListIncidentsFiltered(ctx, domain.IncidentFilter{})
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, "inc-C-pending-recent", all[0].ID, "newest-first")

	// 2) State multi-select.
	statePending, err := s.ListIncidentsFiltered(ctx, domain.IncidentFilter{
		States: []domain.IncidentState{domain.StatePending},
	})
	require.NoError(t, err)
	require.Len(t, statePending, 2)

	// 3) Ecosystem multi-select (post-filter via joined IoC).
	gomodOnly, err := s.ListIncidentsFiltered(ctx, domain.IncidentFilter{
		Ecosystems: []string{"gomod"},
	})
	require.NoError(t, err)
	require.Len(t, gomodOnly, 1)
	require.Equal(t, "inc-C-pending-recent", gomodOnly[0].ID)

	// 4) Time range — only the mid + recent incidents.
	from := now.Add(-2 * time.Hour)
	since2h, err := s.ListIncidentsFiltered(ctx, domain.IncidentFilter{From: &from})
	require.NoError(t, err)
	require.Len(t, since2h, 2)

	// 5) Search across IoC id substring.
	searchGomod, err := s.ListIncidentsFiltered(ctx, domain.IncidentFilter{Search: "gomod"})
	require.NoError(t, err)
	require.Len(t, searchGomod, 1)

	// 6) Owner exact (post-filter via joined Component).
	teamPlatform, err := s.ListIncidentsFiltered(ctx, domain.IncidentFilter{Owner: "team-platform"})
	require.NoError(t, err)
	require.Len(t, teamPlatform, 1)
	require.Equal(t, "inc-B-triaged-mid", teamPlatform[0].ID)

	// 7) Limit.
	limited, err := s.ListIncidentsFiltered(ctx, domain.IncidentFilter{Limit: 1})
	require.NoError(t, err)
	require.Len(t, limited, 1)
}
