package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// Store is an in-process Storage backend. Safe for concurrent use; guarded by
// a single RWMutex. Designed for tests and single-node demo use — SQLite and
// Postgres backends land in Phase 3.
type Store struct {
	mu sync.RWMutex

	components map[string]domain.Component
	sboms      map[string]domain.SBOM
	iocs       map[string]domain.IoC
	incidents  map[string]domain.Incident
	publishers map[publisherKey]domain.Publisher
	profiles   map[publisherKey]domain.PublisherProfile

	// publisherHistory is keyed by package_ref and holds an
	// append-only time-series. nextSnapshotID mirrors the postgres
	// BIGSERIAL so contract tests see the same monotonic-id semantics.
	publisherHistory map[string][]domain.PublisherSnapshot
	nextSnapshotID   int64
}

type publisherKey struct {
	Ecosystem string
	Name      string
}

func New() *Store {
	return &Store{
		components:       map[string]domain.Component{},
		sboms:            map[string]domain.SBOM{},
		iocs:             map[string]domain.IoC{},
		incidents:        map[string]domain.Incident{},
		publishers:       map[publisherKey]domain.Publisher{},
		profiles:         map[publisherKey]domain.PublisherProfile{},
		publisherHistory: map[string][]domain.PublisherSnapshot{},
	}
}

// Compile-time interface check.
var _ storage.Storage = (*Store)(nil)

func (s *Store) UpsertComponent(ctx context.Context, c domain.Component) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.components[c.Ref] = c
	return ctx.Err()
}

func (s *Store) GetComponent(ctx context.Context, ref string) (*domain.Component, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.components[ref]
	if !ok {
		return nil, storage.ErrNotFound
	}
	out := c
	return &out, ctx.Err()
}

func (s *Store) ListComponents(ctx context.Context) ([]domain.Component, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Component, 0, len(s.components))
	for _, c := range s.components {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out, ctx.Err()
}

func (s *Store) UpsertSBOM(ctx context.Context, b domain.SBOM) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sboms[b.ID] = b
	return ctx.Err()
}

func (s *Store) GetSBOM(ctx context.Context, id string) (*domain.SBOM, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.sboms[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	out := b
	return &out, ctx.Err()
}

func (s *Store) ListSBOMsByComponent(ctx context.Context, ref string) ([]domain.SBOM, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.SBOM{}
	for _, b := range s.sboms {
		if b.ComponentRef == ref {
			out = append(out, b)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GeneratedAt.Before(out[j].GeneratedAt) })
	return out, ctx.Err()
}

func (s *Store) UpsertIoC(ctx context.Context, i domain.IoC) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.iocs[i.ID] = i
	return ctx.Err()
}

func (s *Store) GetIoC(ctx context.Context, id string) (*domain.IoC, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.iocs[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	out := i
	return &out, ctx.Err()
}

func (s *Store) ListIoCs(ctx context.Context) ([]domain.IoC, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.IoC, 0, len(s.iocs))
	for _, i := range s.iocs {
		out = append(out, i)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PublishedAt.Before(out[j].PublishedAt) })
	return out, ctx.Err()
}

func (s *Store) UpsertIncident(ctx context.Context, i domain.Incident) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidents[i.ID] = i
	return ctx.Err()
}

func (s *Store) GetIncident(ctx context.Context, id string) (*domain.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.incidents[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	out := i
	return &out, ctx.Err()
}

func (s *Store) ListIncidents(ctx context.Context) ([]domain.Incident, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Incident, 0, len(s.incidents))
	for _, i := range s.incidents {
		out = append(out, i)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.Before(out[j].OpenedAt) })
	return out, ctx.Err()
}

// AppendRemediation atomically appends to an incident's Remediations
// slice. Holds the write lock across the read-modify-write so two
// concurrent appends can't lose an entry.
func (s *Store) AppendRemediation(ctx context.Context, incidentID string, r domain.Remediation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	inc, ok := s.incidents[incidentID]
	if !ok {
		return storage.ErrNotFound
	}
	inc.Remediations = append(inc.Remediations, r)
	s.incidents[incidentID] = inc
	return ctx.Err()
}

func (s *Store) ListRemediations(ctx context.Context, incidentID string) ([]domain.Remediation, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inc, ok := s.incidents[incidentID]
	if !ok {
		return nil, storage.ErrNotFound
	}
	out := make([]domain.Remediation, len(inc.Remediations))
	copy(out, inc.Remediations)
	return out, ctx.Err()
}

func (s *Store) UpsertPublisher(ctx context.Context, p domain.Publisher) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishers[publisherKey{p.Ecosystem, p.Name}] = p
	return ctx.Err()
}

func (s *Store) GetPublisher(ctx context.Context, ecosystem, name string) (*domain.Publisher, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.publishers[publisherKey{ecosystem, name}]
	if !ok {
		return nil, storage.ErrNotFound
	}
	out := p
	return &out, ctx.Err()
}

func (s *Store) UpsertPublisherProfile(ctx context.Context, p domain.PublisherProfile) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.profiles[publisherKey{p.Publisher.Ecosystem, p.Publisher.Name}] = p
	return ctx.Err()
}

func (s *Store) GetPublisherProfile(ctx context.Context, ecosystem, name string) (*domain.PublisherProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.profiles[publisherKey{ecosystem, name}]
	if !ok {
		return nil, storage.ErrNotFound
	}
	out := p
	return &out, ctx.Err()
}

func (s *Store) ListPublishers(ctx context.Context, ecosystem string) ([]domain.Publisher, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []domain.Publisher{}
	for k, p := range s.publishers {
		if k.Ecosystem == ecosystem {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, ctx.Err()
}

// --- PublisherSnapshot --------------------------------------------------

func (s *Store) SavePublisherSnapshot(ctx context.Context, snap domain.PublisherSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextSnapshotID++
	snap.ID = s.nextSnapshotID
	if snap.SnapshotAt.IsZero() {
		snap.SnapshotAt = time.Now().UTC()
	} else {
		snap.SnapshotAt = snap.SnapshotAt.UTC()
	}
	if snap.LatestVersionPublishedAt != nil {
		t := snap.LatestVersionPublishedAt.UTC()
		snap.LatestVersionPublishedAt = &t
	}
	s.publisherHistory[snap.PackageRef] = append(s.publisherHistory[snap.PackageRef], snap)
	return ctx.Err()
}

func (s *Store) GetPublisherHistory(ctx context.Context, packageRef string, limit int) ([]domain.PublisherSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all, ok := s.publisherHistory[packageRef]
	if !ok {
		return []domain.PublisherSnapshot{}, ctx.Err()
	}
	// Copy + sort newest-first; matches the postgres ORDER BY snapshot_at DESC.
	out := make([]domain.PublisherSnapshot, len(all))
	copy(out, all)
	sort.Slice(out, func(i, j int) bool {
		return out[i].SnapshotAt.After(out[j].SnapshotAt)
	})
	if limit > 0 && limit < len(out) {
		out = out[:limit]
	}
	return out, ctx.Err()
}

func (s *Store) ListPackagesNeedingRefresh(ctx context.Context, olderThan time.Time, limit int) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	type cand struct {
		ref    string
		latest time.Time
	}
	cands := make([]cand, 0, len(s.publisherHistory))
	for ref, hist := range s.publisherHistory {
		if len(hist) == 0 {
			continue
		}
		latest := hist[0].SnapshotAt
		for _, sn := range hist[1:] {
			if sn.SnapshotAt.After(latest) {
				latest = sn.SnapshotAt
			}
		}
		if latest.Before(olderThan) {
			cands = append(cands, cand{ref: ref, latest: latest})
		}
	}
	// Stale-first ordering. Postgres returns the same shape with an
	// ORDER BY MAX(snapshot_at) ASC.
	sort.Slice(cands, func(i, j int) bool {
		return cands[i].latest.Before(cands[j].latest)
	})
	out := make([]string, 0, len(cands))
	for _, c := range cands {
		out = append(out, c.ref)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, ctx.Err()
}

func (s *Store) Close() error { return nil }
