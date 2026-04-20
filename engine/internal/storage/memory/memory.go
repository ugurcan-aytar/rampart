package memory

import (
	"context"
	"sort"
	"sync"

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
}

type publisherKey struct {
	Ecosystem string
	Name      string
}

func New() *Store {
	return &Store{
		components: map[string]domain.Component{},
		sboms:      map[string]domain.SBOM{},
		iocs:       map[string]domain.IoC{},
		incidents:  map[string]domain.Incident{},
		publishers: map[publisherKey]domain.Publisher{},
		profiles:   map[publisherKey]domain.PublisherProfile{},
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

func (s *Store) Close() error { return nil }
