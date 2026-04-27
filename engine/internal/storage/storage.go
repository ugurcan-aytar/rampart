package storage

import (
	"context"
	"errors"
	"time"

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

	// ListSBOMPackages returns every (component, package) pair where
	// the package matches (ecosystem, name). Hot path for the matcher:
	// forwardMatch (one query per IoC) and BlastRadius live fallback
	// (one query per what-if IoC) both call this instead of looping
	// ListSBOMsByComponent across every component. Postgres uses the
	// existing sbom_packages_name_version_idx (ecosystem + name
	// prefix) for the WHERE; version-constraint matching stays in
	// matcher.Evaluate so a single storage method serves all three
	// IoC kinds.
	ListSBOMPackages(ctx context.Context, ecosystem, name string) ([]domain.SBOMPackageRef, error)

	// IoC
	UpsertIoC(ctx context.Context, i domain.IoC) error
	GetIoC(ctx context.Context, id string) (*domain.IoC, error)
	ListIoCs(ctx context.Context) ([]domain.IoC, error)

	// Incident
	UpsertIncident(ctx context.Context, i domain.Incident) error
	GetIncident(ctx context.Context, id string) (*domain.Incident, error)
	ListIncidents(ctx context.Context) ([]domain.Incident, error)

	// ListIncidentsFiltered scopes by domain.IncidentFilter — multi-state
	// + ecosystem + time range + substring search + owner. Backends apply
	// the indexed dimensions natively (postgres WHERE / memory iteration)
	// and post-filter the cross-table dimensions (Search across joined
	// IoC ecosystem, Owner across joined Component). Newest-first.
	ListIncidentsFiltered(ctx context.Context, filter domain.IncidentFilter) ([]domain.Incident, error)

	// MatchedComponentRefsByIoC returns the distinct component refs
	// the given IoC has ever opened an incident for, across all
	// incident states (pending → closed). Hot path for the
	// BlastRadius cached lookup; postgres uses the existing
	// incidents_ioc_idx. An empty result means either no match or
	// the IoC was never ingested — callers can fall back to the
	// live matcher path to preserve what-if semantics.
	MatchedComponentRefsByIoC(ctx context.Context, iocID string) ([]string, error)

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

	// PublisherSnapshot — append-only time-series of upstream publisher
	// metadata. Theme F1 ingests these from npm + GitHub APIs; Theme F2
	// detectors diff successive snapshots to raise PublisherSignals.
	SavePublisherSnapshot(ctx context.Context, snapshot domain.PublisherSnapshot) error
	GetPublisherHistory(ctx context.Context, packageRef string, limit int) ([]domain.PublisherSnapshot, error)
	ListPackagesNeedingRefresh(ctx context.Context, olderThan time.Time, limit int) ([]string, error)

	// Anomaly — Theme F2 detector hits. SaveAnomaly is idempotent on
	// (Kind, PackageRef, DetectedAt) — re-running a detector over
	// the same snapshot history is a no-op rather than producing
	// duplicates. GetAnomaly + ListAnomalies serve the
	// `GET /v1/anomalies` surface.
	SaveAnomaly(ctx context.Context, a domain.Anomaly) (int64, error)
	GetAnomaly(ctx context.Context, id int64) (*domain.Anomaly, error)
	ListAnomalies(ctx context.Context, filter domain.AnomalyFilter) ([]domain.Anomaly, error)

	Close() error
}
