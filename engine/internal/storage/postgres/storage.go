package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// Store is a Postgres-backed Storage implementation. Concurrent-safe by
// virtue of pgxpool; the pool handles connection lifecycle and any
// per-request transactions are short and committed before the method
// returns. The store is strictly schema-aware — it does not call
// MigrateUp on its own (callers do that at boot so failures surface
// before `/readyz` returns OK).
type Store struct {
	pool *pgxpool.Pool
}

// Compile-time interface check.
var _ storage.Storage = (*Store)(nil)

// Open constructs a Store from a Postgres DSN. The caller is
// responsible for running MigrateUp against the same DSN before using
// the store; Open itself does not touch the schema.
func Open(ctx context.Context, dsn string, maxConns int32) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: new pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	return &Store{pool: pool}, nil
}

// Close releases the pool. Safe to call multiple times.
func (s *Store) Close() error {
	if s.pool != nil {
		s.pool.Close()
		s.pool = nil
	}
	return nil
}

// --- Components ------------------------------------------------------------

func (s *Store) UpsertComponent(ctx context.Context, c domain.Component) error {
	ann, err := marshalJSON(c.Annotations)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO components
		    (ref, kind, namespace, name, owner, system, lifecycle, tags, annotations)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (ref) DO UPDATE SET
		    kind        = EXCLUDED.kind,
		    namespace   = EXCLUDED.namespace,
		    name        = EXCLUDED.name,
		    owner       = EXCLUDED.owner,
		    system      = EXCLUDED.system,
		    lifecycle   = EXCLUDED.lifecycle,
		    tags        = EXCLUDED.tags,
		    annotations = EXCLUDED.annotations`,
		c.Ref, c.Kind, c.Namespace, c.Name, c.Owner, c.System, c.Lifecycle,
		nonNilStrings(c.Tags), ann,
	)
	return wrapPgErr(err, "UpsertComponent")
}

func (s *Store) GetComponent(ctx context.Context, ref string) (*domain.Component, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT ref, kind, namespace, name, owner, system, lifecycle, tags, annotations
		FROM components WHERE ref = $1`, ref)
	c, err := scanComponent(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: GetComponent: %w", err)
	}
	return c, nil
}

func (s *Store) ListComponents(ctx context.Context) ([]domain.Component, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ref, kind, namespace, name, owner, system, lifecycle, tags, annotations
		FROM components ORDER BY ref ASC`)
	if err != nil {
		return nil, fmt.Errorf("postgres: ListComponents: %w", err)
	}
	defer rows.Close()
	out := []domain.Component{}
	for rows.Next() {
		c, err := scanComponent(rows)
		if err != nil {
			return nil, fmt.Errorf("postgres: ListComponents scan: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// rowScanner is the shared slice of Row / Rows needed to read a single
// row — lets scanComponent service both QueryRow and Query callers.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanComponent(r rowScanner) (*domain.Component, error) {
	var c domain.Component
	var tags []string
	var ann []byte
	if err := r.Scan(
		&c.Ref, &c.Kind, &c.Namespace, &c.Name,
		&c.Owner, &c.System, &c.Lifecycle, &tags, &ann,
	); err != nil {
		return nil, err
	}
	c.Tags = tags
	if err := unmarshalJSON(ann, &c.Annotations); err != nil {
		return nil, err
	}
	return &c, nil
}

// --- SBOM ------------------------------------------------------------------

func (s *Store) UpsertSBOM(ctx context.Context, b domain.SBOM) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("postgres: UpsertSBOM begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO sboms
		    (id, component_ref, commit_sha, ecosystem, generated_at, source_format, source_bytes)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
		    component_ref = EXCLUDED.component_ref,
		    commit_sha    = EXCLUDED.commit_sha,
		    ecosystem     = EXCLUDED.ecosystem,
		    generated_at  = EXCLUDED.generated_at,
		    source_format = EXCLUDED.source_format,
		    source_bytes  = EXCLUDED.source_bytes`,
		b.ID, b.ComponentRef, b.CommitSHA, b.Ecosystem, b.GeneratedAt.UTC(),
		b.SourceFormat, b.SourceBytes,
	); err != nil {
		return wrapPgErr(err, "UpsertSBOM")
	}
	// Re-write the package list — SBOMs are immutable at the domain
	// level, but the upsert keeps the backend lenient: delete + insert
	// yields the same post-condition as insert-only when the caller
	// passes the same (id, packages) pair.
	if _, err := tx.Exec(ctx, `DELETE FROM sbom_packages WHERE sbom_id = $1`, b.ID); err != nil {
		return fmt.Errorf("postgres: UpsertSBOM clear packages: %w", err)
	}
	for i, p := range b.Packages {
		if _, err := tx.Exec(ctx, `
			INSERT INTO sbom_packages
			    (sbom_id, position, ecosystem, name, version, purl, scope, integrity)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			b.ID, i, p.Ecosystem, p.Name, p.Version, p.PURL,
			nonNilStrings(p.Scope), p.Integrity,
		); err != nil {
			return fmt.Errorf("postgres: UpsertSBOM insert package: %w", err)
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) GetSBOM(ctx context.Context, id string) (*domain.SBOM, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, component_ref, commit_sha, ecosystem, generated_at, source_format, source_bytes
		FROM sboms WHERE id = $1`, id)
	sb, err := scanSBOM(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: GetSBOM: %w", err)
	}
	if err := s.hydrateSBOMPackages(ctx, sb); err != nil {
		return nil, err
	}
	return sb, nil
}

func (s *Store) ListSBOMsByComponent(ctx context.Context, ref string) ([]domain.SBOM, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, component_ref, commit_sha, ecosystem, generated_at, source_format, source_bytes
		FROM sboms WHERE component_ref = $1 ORDER BY generated_at ASC`, ref)
	if err != nil {
		return nil, fmt.Errorf("postgres: ListSBOMsByComponent: %w", err)
	}
	defer rows.Close()
	out := []domain.SBOM{}
	for rows.Next() {
		sb, err := scanSBOM(rows)
		if err != nil {
			return nil, err
		}
		if err := s.hydrateSBOMPackages(ctx, sb); err != nil {
			return nil, err
		}
		out = append(out, *sb)
	}
	return out, rows.Err()
}

func scanSBOM(r rowScanner) (*domain.SBOM, error) {
	var b domain.SBOM
	if err := r.Scan(
		&b.ID, &b.ComponentRef, &b.CommitSHA, &b.Ecosystem,
		&b.GeneratedAt, &b.SourceFormat, &b.SourceBytes,
	); err != nil {
		return nil, err
	}
	b.GeneratedAt = b.GeneratedAt.UTC()
	return &b, nil
}

func (s *Store) hydrateSBOMPackages(ctx context.Context, b *domain.SBOM) error {
	rows, err := s.pool.Query(ctx, `
		SELECT ecosystem, name, version, purl, scope, integrity
		FROM sbom_packages WHERE sbom_id = $1 ORDER BY position ASC`, b.ID)
	if err != nil {
		return fmt.Errorf("postgres: hydrateSBOMPackages: %w", err)
	}
	defer rows.Close()
	b.Packages = nil
	for rows.Next() {
		var p domain.PackageVersion
		var scope []string
		if err := rows.Scan(&p.Ecosystem, &p.Name, &p.Version, &p.PURL, &scope, &p.Integrity); err != nil {
			return err
		}
		p.Scope = scope
		b.Packages = append(b.Packages, p)
	}
	return rows.Err()
}

// --- IoC -------------------------------------------------------------------

// iocBody captures the tagged-union payload. Exactly one of the three
// pointer fields is populated on the wire; JSONB does the
// serialization. Keeping the body in one column (rather than three
// flat tables) keeps the migration slim and the hot GetIoC path to a
// single row scan.
type iocBody struct {
	PackageVersion   *domain.IoCPackageVersion   `json:"packageVersion,omitempty"`
	PackageRange     *domain.IoCPackageRange     `json:"packageRange,omitempty"`
	PublisherAnomaly *domain.IoCPublisherAnomaly `json:"publisherAnomaly,omitempty"`
}

func (s *Store) UpsertIoC(ctx context.Context, i domain.IoC) error {
	body := iocBody{
		PackageVersion:   i.PackageVersion,
		PackageRange:     i.PackageRange,
		PublisherAnomaly: i.PublisherAnomaly,
	}
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("postgres: UpsertIoC marshal body: %w", err)
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO iocs
		    (id, kind, severity, ecosystem, source, published_at, description, body)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
		    kind         = EXCLUDED.kind,
		    severity     = EXCLUDED.severity,
		    ecosystem    = EXCLUDED.ecosystem,
		    source       = EXCLUDED.source,
		    published_at = EXCLUDED.published_at,
		    description  = EXCLUDED.description,
		    body         = EXCLUDED.body`,
		i.ID, string(i.Kind), string(i.Severity), i.Ecosystem, i.Source,
		i.PublishedAt.UTC(), i.Description, b,
	)
	return wrapPgErr(err, "UpsertIoC")
}

func (s *Store) GetIoC(ctx context.Context, id string) (*domain.IoC, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, kind, severity, ecosystem, source, published_at, description, body
		FROM iocs WHERE id = $1`, id)
	i, err := scanIoC(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: GetIoC: %w", err)
	}
	return i, nil
}

func (s *Store) ListIoCs(ctx context.Context) ([]domain.IoC, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, kind, severity, ecosystem, source, published_at, description, body
		FROM iocs ORDER BY published_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("postgres: ListIoCs: %w", err)
	}
	defer rows.Close()
	out := []domain.IoC{}
	for rows.Next() {
		i, err := scanIoC(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

func scanIoC(r rowScanner) (*domain.IoC, error) {
	var i domain.IoC
	var kind, severity string
	var body []byte
	if err := r.Scan(
		&i.ID, &kind, &severity, &i.Ecosystem, &i.Source,
		&i.PublishedAt, &i.Description, &body,
	); err != nil {
		return nil, err
	}
	i.Kind = domain.IoCKind(kind)
	i.Severity = domain.Severity(severity)
	i.PublishedAt = i.PublishedAt.UTC()
	var parsed iocBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("postgres: decode ioc body: %w", err)
	}
	i.PackageVersion = parsed.PackageVersion
	i.PackageRange = parsed.PackageRange
	i.PublisherAnomaly = parsed.PublisherAnomaly
	return &i, nil
}

// --- Incident --------------------------------------------------------------

func (s *Store) UpsertIncident(ctx context.Context, i domain.Incident) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO incidents
		    (id, ioc_id, state, opened_at, last_transitioned_at, affected_components_snapshot)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
		    ioc_id                       = EXCLUDED.ioc_id,
		    state                        = EXCLUDED.state,
		    opened_at                    = EXCLUDED.opened_at,
		    last_transitioned_at         = EXCLUDED.last_transitioned_at,
		    affected_components_snapshot = EXCLUDED.affected_components_snapshot`,
		i.ID, i.IoCID, string(i.State), i.OpenedAt.UTC(), i.LastTransitionedAt.UTC(),
		nonNilStrings(i.AffectedComponentsSnapshot),
	)
	return wrapPgErr(err, "UpsertIncident")
}

func (s *Store) GetIncident(ctx context.Context, id string) (*domain.Incident, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, ioc_id, state, opened_at, last_transitioned_at, affected_components_snapshot
		FROM incidents WHERE id = $1`, id)
	i, err := scanIncident(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: GetIncident: %w", err)
	}
	rems, err := s.listRemediationsTx(ctx, s.pool, id)
	if err != nil {
		return nil, err
	}
	i.Remediations = rems
	return i, nil
}

func (s *Store) ListIncidents(ctx context.Context) ([]domain.Incident, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, ioc_id, state, opened_at, last_transitioned_at, affected_components_snapshot
		FROM incidents ORDER BY opened_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("postgres: ListIncidents: %w", err)
	}
	defer rows.Close()
	out := []domain.Incident{}
	for rows.Next() {
		i, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

func scanIncident(r rowScanner) (*domain.Incident, error) {
	var i domain.Incident
	var state string
	var snapshot []string
	if err := r.Scan(
		&i.ID, &i.IoCID, &state, &i.OpenedAt, &i.LastTransitionedAt, &snapshot,
	); err != nil {
		return nil, err
	}
	i.State = domain.IncidentState(state)
	i.OpenedAt = i.OpenedAt.UTC()
	i.LastTransitionedAt = i.LastTransitionedAt.UTC()
	i.AffectedComponentsSnapshot = snapshot
	return &i, nil
}

// --- Remediation -----------------------------------------------------------

// remediationExecutor is the small subset of pgxpool.Pool / pgx.Tx the
// helpers use so `listRemediationsTx` can serve both the standalone
// `ListRemediations` call and the inline hydration inside
// `GetIncident`.
type remediationExecutor interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

func (s *Store) AppendRemediation(ctx context.Context, incidentID string, r domain.Remediation) error {
	// Confirm the incident exists first — the storagetest contract
	// expects storage.ErrNotFound on AppendRemediation against a
	// missing incident. Foreign-key violation would raise a different
	// sentinel, so check explicitly before inserting.
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM incidents WHERE id = $1)`, incidentID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("postgres: AppendRemediation exists: %w", err)
	}
	if !exists {
		return storage.ErrNotFound
	}
	details, err := marshalJSON(r.Details)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO remediations
		    (id, incident_id, kind, executed_at, actor_ref, details)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
		    kind        = EXCLUDED.kind,
		    executed_at = EXCLUDED.executed_at,
		    actor_ref   = EXCLUDED.actor_ref,
		    details     = EXCLUDED.details`,
		r.ID, incidentID, string(r.Kind), r.ExecutedAt.UTC(), r.ActorRef, details,
	)
	return wrapPgErr(err, "AppendRemediation")
}

func (s *Store) ListRemediations(ctx context.Context, incidentID string) ([]domain.Remediation, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM incidents WHERE id = $1)`, incidentID,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("postgres: ListRemediations exists: %w", err)
	}
	if !exists {
		return nil, storage.ErrNotFound
	}
	return s.listRemediationsTx(ctx, s.pool, incidentID)
}

func (s *Store) listRemediationsTx(ctx context.Context, ex remediationExecutor, incidentID string) ([]domain.Remediation, error) {
	rows, err := ex.Query(ctx, `
		SELECT id, incident_id, kind, executed_at, actor_ref, details
		FROM remediations
		WHERE incident_id = $1
		ORDER BY seq ASC`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("postgres: listRemediations: %w", err)
	}
	defer rows.Close()
	out := []domain.Remediation{}
	for rows.Next() {
		var r domain.Remediation
		var kind string
		var details []byte
		if err := rows.Scan(&r.ID, &r.IncidentID, &kind, &r.ExecutedAt, &r.ActorRef, &details); err != nil {
			return nil, err
		}
		r.Kind = domain.RemediationKind(kind)
		r.ExecutedAt = r.ExecutedAt.UTC()
		if err := unmarshalJSON(details, &r.Details); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Publisher -------------------------------------------------------------

func (s *Store) UpsertPublisher(ctx context.Context, p domain.Publisher) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO publishers (ecosystem, name, first_seen, last_seen)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (ecosystem, name) DO UPDATE SET
		    first_seen = EXCLUDED.first_seen,
		    last_seen  = EXCLUDED.last_seen`,
		p.Ecosystem, p.Name, p.FirstSeen.UTC(), p.LastSeen.UTC(),
	)
	return wrapPgErr(err, "UpsertPublisher")
}

func (s *Store) GetPublisher(ctx context.Context, ecosystem, name string) (*domain.Publisher, error) {
	var p domain.Publisher
	err := s.pool.QueryRow(ctx, `
		SELECT ecosystem, name, first_seen, last_seen
		FROM publishers WHERE ecosystem = $1 AND name = $2`,
		ecosystem, name,
	).Scan(&p.Ecosystem, &p.Name, &p.FirstSeen, &p.LastSeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: GetPublisher: %w", err)
	}
	p.FirstSeen = p.FirstSeen.UTC()
	p.LastSeen = p.LastSeen.UTC()
	return &p, nil
}

func (s *Store) UpsertPublisherProfile(ctx context.Context, pf domain.PublisherProfile) error {
	// PublisherProfile has a FK to Publishers; upsert the parent first
	// so tests that write profiles without a prior UpsertPublisher
	// still succeed. The contract suite does write the publisher first,
	// but a defensive upsert keeps the backend tolerant.
	if err := s.UpsertPublisher(ctx, pf.Publisher); err != nil {
		return err
	}
	var lastChange any
	if pf.LastEmailChange != nil {
		lastChange = pf.LastEmailChange.UTC()
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO publisher_profiles
		    (ecosystem, name, package_count, publish_count, last_30day_publishes,
		     uses_oidc, has_git_tags, maintainer_emails, last_email_change)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (ecosystem, name) DO UPDATE SET
		    package_count        = EXCLUDED.package_count,
		    publish_count        = EXCLUDED.publish_count,
		    last_30day_publishes = EXCLUDED.last_30day_publishes,
		    uses_oidc            = EXCLUDED.uses_oidc,
		    has_git_tags         = EXCLUDED.has_git_tags,
		    maintainer_emails    = EXCLUDED.maintainer_emails,
		    last_email_change    = EXCLUDED.last_email_change`,
		pf.Publisher.Ecosystem, pf.Publisher.Name,
		pf.PackageCount, pf.PublishCount, pf.Last30DayPublishes,
		pf.UsesOIDC, pf.HasGitTags, nonNilStrings(pf.MaintainerEmails),
		lastChange,
	)
	return wrapPgErr(err, "UpsertPublisherProfile")
}

func (s *Store) GetPublisherProfile(ctx context.Context, ecosystem, name string) (*domain.PublisherProfile, error) {
	var pf domain.PublisherProfile
	var emails []string
	var lastChange *time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT pp.package_count, pp.publish_count, pp.last_30day_publishes,
		       pp.uses_oidc, pp.has_git_tags, pp.maintainer_emails, pp.last_email_change,
		       p.ecosystem, p.name, p.first_seen, p.last_seen
		FROM publisher_profiles pp
		JOIN publishers p USING (ecosystem, name)
		WHERE pp.ecosystem = $1 AND pp.name = $2`,
		ecosystem, name,
	).Scan(
		&pf.PackageCount, &pf.PublishCount, &pf.Last30DayPublishes,
		&pf.UsesOIDC, &pf.HasGitTags, &emails, &lastChange,
		&pf.Publisher.Ecosystem, &pf.Publisher.Name,
		&pf.Publisher.FirstSeen, &pf.Publisher.LastSeen,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: GetPublisherProfile: %w", err)
	}
	pf.MaintainerEmails = emails
	pf.Publisher.FirstSeen = pf.Publisher.FirstSeen.UTC()
	pf.Publisher.LastSeen = pf.Publisher.LastSeen.UTC()
	if lastChange != nil {
		u := lastChange.UTC()
		pf.LastEmailChange = &u
	}
	return &pf, nil
}

func (s *Store) ListPublishers(ctx context.Context, ecosystem string) ([]domain.Publisher, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ecosystem, name, first_seen, last_seen
		FROM publishers WHERE ecosystem = $1 ORDER BY name ASC`, ecosystem)
	if err != nil {
		return nil, fmt.Errorf("postgres: ListPublishers: %w", err)
	}
	defer rows.Close()
	out := []domain.Publisher{}
	for rows.Next() {
		var p domain.Publisher
		if err := rows.Scan(&p.Ecosystem, &p.Name, &p.FirstSeen, &p.LastSeen); err != nil {
			return nil, err
		}
		p.FirstSeen = p.FirstSeen.UTC()
		p.LastSeen = p.LastSeen.UTC()
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- PublisherSnapshot ----------------------------------------------------

func (s *Store) SavePublisherSnapshot(ctx context.Context, snap domain.PublisherSnapshot) error {
	maintainersJSON, err := marshalJSON(snap.Maintainers)
	if err != nil {
		return err
	}
	rawData := snap.RawData
	if len(rawData) == 0 {
		rawData = []byte(`null`)
	}
	snapAt := snap.SnapshotAt
	if snapAt.IsZero() {
		snapAt = time.Now().UTC()
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO publisher_history
		    (package_ref, snapshot_at, maintainers, latest_version,
		     latest_version_published_at, publish_method, source_repo_url, raw_data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		snap.PackageRef, snapAt, maintainersJSON,
		nullableString(snap.LatestVersion), snap.LatestVersionPublishedAt,
		nullableString(snap.PublishMethod), snap.SourceRepoURL,
		rawData,
	)
	return wrapPgErr(err, "SavePublisherSnapshot")
}

func (s *Store) GetPublisherHistory(ctx context.Context, packageRef string, limit int) ([]domain.PublisherSnapshot, error) {
	// limit ≤ 0 means "no cap" — translate to a large pg-friendly bound
	// rather than skipping the LIMIT clause, so the query plan is stable.
	const noLimitSentinel = 1 << 31
	effectiveLimit := limit
	if effectiveLimit <= 0 {
		effectiveLimit = noLimitSentinel
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, package_ref, snapshot_at, maintainers, latest_version,
		       latest_version_published_at, publish_method, source_repo_url, raw_data
		FROM publisher_history
		WHERE package_ref = $1
		ORDER BY snapshot_at DESC
		LIMIT $2`, packageRef, effectiveLimit)
	if err != nil {
		return nil, fmt.Errorf("postgres: GetPublisherHistory: %w", err)
	}
	defer rows.Close()
	out := []domain.PublisherSnapshot{}
	for rows.Next() {
		var (
			snap          domain.PublisherSnapshot
			maintainersB  []byte
			rawDataB      []byte
			latestVer     *string
			latestVerPub  *time.Time
			publishMethod *string
			sourceRepoURL *string
		)
		if err := rows.Scan(
			&snap.ID, &snap.PackageRef, &snap.SnapshotAt, &maintainersB,
			&latestVer, &latestVerPub, &publishMethod, &sourceRepoURL, &rawDataB,
		); err != nil {
			return nil, err
		}
		snap.SnapshotAt = snap.SnapshotAt.UTC()
		if err := unmarshalJSON(maintainersB, &snap.Maintainers); err != nil {
			return nil, err
		}
		if latestVer != nil {
			snap.LatestVersion = *latestVer
		}
		if latestVerPub != nil {
			t := latestVerPub.UTC()
			snap.LatestVersionPublishedAt = &t
		}
		if publishMethod != nil {
			snap.PublishMethod = *publishMethod
		}
		snap.SourceRepoURL = sourceRepoURL
		if len(rawDataB) > 0 && string(rawDataB) != "null" {
			snap.RawData = rawDataB
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

func (s *Store) ListPackagesNeedingRefresh(ctx context.Context, olderThan time.Time, limit int) ([]string, error) {
	const noLimitSentinel = 1 << 31
	effectiveLimit := limit
	if effectiveLimit <= 0 {
		effectiveLimit = noLimitSentinel
	}
	// MAX(snapshot_at) collapses each package's history to its newest
	// row; HAVING filters to packages whose latest snapshot is older than
	// the threshold; ORDER BY ASC puts the stalest first.
	rows, err := s.pool.Query(ctx, `
		SELECT package_ref
		FROM publisher_history
		GROUP BY package_ref
		HAVING MAX(snapshot_at) < $1
		ORDER BY MAX(snapshot_at) ASC
		LIMIT $2`, olderThan, effectiveLimit)
	if err != nil {
		return nil, fmt.Errorf("postgres: ListPackagesNeedingRefresh: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}

// nullableString returns nil for empty strings so optional TEXT
// columns store NULL instead of an empty value (matches the in-memory
// "absent → nil" semantics).
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// --- Anomaly --------------------------------------------------------------

func (s *Store) SaveAnomaly(ctx context.Context, a domain.Anomaly) (int64, error) {
	evidenceJSON, err := marshalJSON(a.Evidence)
	if err != nil {
		return 0, err
	}
	detectedAt := a.DetectedAt
	if detectedAt.IsZero() {
		detectedAt = time.Now().UTC()
	}
	// ON CONFLICT (kind, package_ref, detected_at) DO UPDATE SET id=id
	// is the idiomatic upsert that returns the existing row's id when
	// the dedup key already exists — without DO UPDATE the RETURNING
	// clause yields no row on conflict.
	var id int64
	err = s.pool.QueryRow(ctx, `
		INSERT INTO anomalies
		    (kind, package_ref, detected_at, confidence, explanation, evidence)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (kind, package_ref, detected_at) DO UPDATE SET id = anomalies.id
		RETURNING id`,
		string(a.Kind), a.PackageRef, detectedAt, string(a.Confidence),
		a.Explanation, evidenceJSON,
	).Scan(&id)
	if err != nil {
		return 0, wrapPgErr(err, "SaveAnomaly")
	}
	return id, nil
}

func (s *Store) GetAnomaly(ctx context.Context, id int64) (*domain.Anomaly, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, kind, package_ref, detected_at, confidence, explanation, evidence
		FROM anomalies WHERE id = $1`, id)
	a, err := scanAnomaly(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, storage.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("postgres: GetAnomaly: %w", err)
	}
	return a, nil
}

func (s *Store) ListAnomalies(ctx context.Context, filter domain.AnomalyFilter) ([]domain.Anomaly, error) {
	const noLimitSentinel = 1 << 31
	limit := filter.Limit
	if limit <= 0 {
		limit = noLimitSentinel
	}
	// We pass each filter dimension as a nullable parameter so the
	// query plan stays the same regardless of which slots are set.
	// COALESCE-style guards in the WHERE clause turn nil → "match all".
	var (
		pkgRef *string
		kind   *string
		from   *time.Time
		to     *time.Time
	)
	if filter.PackageRef != "" {
		v := filter.PackageRef
		pkgRef = &v
	}
	if filter.Kind != "" {
		v := string(filter.Kind)
		kind = &v
	}
	from = filter.From
	to = filter.To

	rows, err := s.pool.Query(ctx, `
		SELECT id, kind, package_ref, detected_at, confidence, explanation, evidence
		FROM anomalies
		WHERE ($1::text IS NULL OR package_ref = $1)
		  AND ($2::text IS NULL OR kind = $2)
		  AND ($3::timestamptz IS NULL OR detected_at >= $3)
		  AND ($4::timestamptz IS NULL OR detected_at <= $4)
		ORDER BY detected_at DESC
		LIMIT $5`, pkgRef, kind, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("postgres: ListAnomalies: %w", err)
	}
	defer rows.Close()
	out := []domain.Anomaly{}
	for rows.Next() {
		a, err := scanAnomaly(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// scanAnomaly accepts both *pgx.Row and pgx.Rows via the existing
// rowScanner interface defined alongside scanComponent.
func scanAnomaly(r rowScanner) (*domain.Anomaly, error) {
	var (
		a            domain.Anomaly
		kind, conf   string
		evidenceJSON []byte
	)
	if err := r.Scan(&a.ID, &kind, &a.PackageRef, &a.DetectedAt, &conf, &a.Explanation, &evidenceJSON); err != nil {
		return nil, err
	}
	a.Kind = domain.AnomalyKind(kind)
	a.Confidence = domain.ConfidenceLevel(conf)
	a.DetectedAt = a.DetectedAt.UTC()
	if err := unmarshalJSON(evidenceJSON, &a.Evidence); err != nil {
		return nil, err
	}
	return &a, nil
}

// --- helpers ---------------------------------------------------------------

// nonNilStrings returns an empty slice for nil so pgx's array encoder
// produces `{}` instead of NULL — TEXT[] columns are NOT NULL in the
// schema, and that matches the in-memory "no tags" semantics.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func marshalJSON(v any) ([]byte, error) {
	if v == nil {
		return []byte(`{}`), nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("postgres: marshal json: %w", err)
	}
	return b, nil
}

func unmarshalJSON(b []byte, out any) error {
	if len(b) == 0 || string(b) == `{}` {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("postgres: unmarshal json: %w", err)
	}
	return nil
}

func wrapPgErr(err error, op string) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Errorf("postgres: %s: %s (code %s)", op, pgErr.Message, pgErr.Code)
	}
	return fmt.Errorf("postgres: %s: %w", op, err)
}
