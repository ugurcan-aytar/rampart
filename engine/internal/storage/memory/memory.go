package memory

import (
	"context"
	"sort"
	"strings"
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

	// anomalies are keyed by id (BIGSERIAL-style); the dedup index
	// mirrors the postgres UNIQUE (kind, package_ref, detected_at)
	// constraint so re-runs from a detector are idempotent.
	anomalies     map[int64]domain.Anomaly
	anomalyDedup  map[anomalyDedupKey]int64
	nextAnomalyID int64
}

type anomalyDedupKey struct {
	Kind       string
	PackageRef string
	DetectedAt time.Time
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
		anomalies:        map[int64]domain.Anomaly{},
		anomalyDedup:     map[anomalyDedupKey]int64{},
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

func (s *Store) ListIncidentsFiltered(ctx context.Context, f domain.IncidentFilter) ([]domain.Incident, error) {
	s.mu.RLock()
	candidates := make([]domain.Incident, 0, len(s.incidents))
	for _, inc := range s.incidents {
		if !matchesIndexedFilter(inc, f) {
			continue
		}
		candidates = append(candidates, inc)
	}
	// Snapshot the joined-data lookups while still holding the read
	// lock so we don't race against a concurrent UpsertIoC.
	iocsCopy := make(map[string]domain.IoC, len(s.iocs))
	for k, v := range s.iocs {
		iocsCopy[k] = v
	}
	componentsCopy := make(map[string]domain.Component, len(s.components))
	for k, v := range s.components {
		componentsCopy[k] = v
	}
	s.mu.RUnlock()

	out := make([]domain.Incident, 0, len(candidates))
	for _, inc := range candidates {
		if !matchesEcosystemFilter(inc, f, iocsCopy) {
			continue
		}
		if !matchesSearchFilter(inc, f) {
			continue
		}
		if !matchesOwnerFilter(inc, f, componentsCopy) {
			continue
		}
		out = append(out, inc)
	}
	// Newest-first; matches postgres ORDER BY opened_at DESC.
	sort.Slice(out, func(i, j int) bool { return out[i].OpenedAt.After(out[j].OpenedAt) })
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, ctx.Err()
}

// matchesIndexedFilter checks the dimensions the postgres equivalent
// covers in its WHERE clause: state set, time-range, id substring.
func matchesIndexedFilter(inc domain.Incident, f domain.IncidentFilter) bool {
	if len(f.States) > 0 {
		ok := false
		for _, st := range f.States {
			if inc.State == st {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if f.From != nil && inc.OpenedAt.Before(*f.From) {
		return false
	}
	if f.To != nil && inc.OpenedAt.After(*f.To) {
		return false
	}
	return true
}

func matchesEcosystemFilter(inc domain.Incident, f domain.IncidentFilter, iocs map[string]domain.IoC) bool {
	if len(f.Ecosystems) == 0 {
		return true
	}
	ioc, ok := iocs[inc.IoCID]
	if !ok {
		return false
	}
	for _, eco := range f.Ecosystems {
		if eco != "" && ioc.Ecosystem == eco {
			return true
		}
	}
	return false
}

func matchesSearchFilter(inc domain.Incident, f domain.IncidentFilter) bool {
	if f.Search == "" {
		return true
	}
	q := strings.ToLower(f.Search)
	if strings.Contains(strings.ToLower(inc.ID), q) {
		return true
	}
	if strings.Contains(strings.ToLower(inc.IoCID), q) {
		return true
	}
	for _, ref := range inc.AffectedComponentsSnapshot {
		if strings.Contains(strings.ToLower(ref), q) {
			return true
		}
	}
	return false
}

func matchesOwnerFilter(inc domain.Incident, f domain.IncidentFilter, components map[string]domain.Component) bool {
	if f.Owner == "" {
		return true
	}
	for _, ref := range inc.AffectedComponentsSnapshot {
		c, ok := components[ref]
		if !ok {
			continue
		}
		if c.Owner == f.Owner {
			return true
		}
	}
	return false
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

// --- Anomaly ------------------------------------------------------------

func (s *Store) SaveAnomaly(ctx context.Context, a domain.Anomaly) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a.DetectedAt.IsZero() {
		a.DetectedAt = time.Now().UTC()
	} else {
		a.DetectedAt = a.DetectedAt.UTC()
	}
	dk := anomalyDedupKey{Kind: string(a.Kind), PackageRef: a.PackageRef, DetectedAt: a.DetectedAt}
	if existing, ok := s.anomalyDedup[dk]; ok {
		return existing, ctx.Err()
	}
	s.nextAnomalyID++
	a.ID = s.nextAnomalyID
	s.anomalies[a.ID] = a
	s.anomalyDedup[dk] = a.ID
	return a.ID, ctx.Err()
}

func (s *Store) GetAnomaly(ctx context.Context, id int64) (*domain.Anomaly, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.anomalies[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	out := a
	return &out, ctx.Err()
}

func (s *Store) ListAnomalies(ctx context.Context, filter domain.AnomalyFilter) ([]domain.Anomaly, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]domain.Anomaly, 0)
	for _, a := range s.anomalies {
		if filter.PackageRef != "" && a.PackageRef != filter.PackageRef {
			continue
		}
		if filter.Kind != "" && a.Kind != filter.Kind {
			continue
		}
		if filter.From != nil && a.DetectedAt.Before(*filter.From) {
			continue
		}
		if filter.To != nil && a.DetectedAt.After(*filter.To) {
			continue
		}
		out = append(out, a)
	}
	// Newest-first; matches postgres ORDER BY detected_at DESC.
	sort.Slice(out, func(i, j int) bool {
		return out[i].DetectedAt.After(out[j].DetectedAt)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, ctx.Err()
}

func (s *Store) Close() error { return nil }
