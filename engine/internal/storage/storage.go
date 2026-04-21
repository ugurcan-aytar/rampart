package storage

import (
	"context"
	"errors"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// ErrNotFound is returned by Get* methods when the record does not exist.
var ErrNotFound = errors.New("not found")

// Storage is the persistence contract every backend implements. Backends pass
// the shared suite in storagetest.Run to claim compliance.
type Storage interface {
	// Component
	UpsertComponent(ctx context.Context, c domain.Component) error
	GetComponent(ctx context.Context, ref string) (*domain.Component, error)
	ListComponents(ctx context.Context) ([]domain.Component, error)

	// SBOM
	UpsertSBOM(ctx context.Context, s domain.SBOM) error
	GetSBOM(ctx context.Context, id string) (*domain.SBOM, error)
	ListSBOMsByComponent(ctx context.Context, ref string) ([]domain.SBOM, error)

	// IoC
	UpsertIoC(ctx context.Context, i domain.IoC) error
	GetIoC(ctx context.Context, id string) (*domain.IoC, error)
	ListIoCs(ctx context.Context) ([]domain.IoC, error)

	// Incident
	UpsertIncident(ctx context.Context, i domain.Incident) error
	GetIncident(ctx context.Context, id string) (*domain.Incident, error)
	ListIncidents(ctx context.Context) ([]domain.Incident, error)

	// Remediation — append-only audit log. Each entry is attached to an
	// Incident; storage backends are expected to append atomically so
	// concurrent actors don't clobber each other's entries.
	AppendRemediation(ctx context.Context, incidentID string, r domain.Remediation) error
	ListRemediations(ctx context.Context, incidentID string) ([]domain.Remediation, error)

	// Publisher
	UpsertPublisher(ctx context.Context, p domain.Publisher) error
	GetPublisher(ctx context.Context, ecosystem, name string) (*domain.Publisher, error)
	UpsertPublisherProfile(ctx context.Context, p domain.PublisherProfile) error
	GetPublisherProfile(ctx context.Context, ecosystem, name string) (*domain.PublisherProfile, error)
	ListPublishers(ctx context.Context, ecosystem string) ([]domain.Publisher, error)

	Close() error
}
