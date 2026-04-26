package api

import (
	"net/http"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// defaultHistoryLimit caps a single GetPublisherHistory response when
// the caller does not supply ?limit=. The OpenAPI schema documents 100
// as the default; keep them in sync.
const defaultHistoryLimit = 100

// GetPublisherHistory implements
// `GET /v1/publisher/{packageRef}/history`. Returns the snapshots
// newest-first; an unknown packageRef returns an empty array (200),
// not 404 — F2 detectors poll-then-diff and a 404 would force them
// to special-case "no history yet".
func (s *Server) GetPublisherHistory(w http.ResponseWriter, r *http.Request, packageRef string, params gen.GetPublisherHistoryParams) {
	limit := defaultHistoryLimit
	if params.Limit != nil {
		limit = *params.Limit
	}
	if packageRef == "" {
		writeError(w, http.StatusBadRequest, "INVALID_PACKAGE_REF",
			"packageRef path parameter must be non-empty")
		return
	}

	rows, err := s.storage.GetPublisherHistory(r.Context(), packageRef, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	out := gen.PublisherHistory{Items: make([]gen.PublisherSnapshot, 0, len(rows))}
	for _, snap := range rows {
		out.Items = append(out.Items, snapshotToWire(snap))
	}
	writeJSON(w, http.StatusOK, out)
}

// snapshotToWire mirrors the domain PublisherSnapshot onto the
// generated OpenAPI shape. RawData is intentionally not surfaced —
// see the schema comment for rationale.
func snapshotToWire(snap domain.PublisherSnapshot) gen.PublisherSnapshot {
	out := gen.PublisherSnapshot{
		Id:         snap.ID,
		PackageRef: snap.PackageRef,
		SnapshotAt: snap.SnapshotAt,
	}
	if len(snap.Maintainers) > 0 {
		ms := make([]gen.Maintainer, 0, len(snap.Maintainers))
		for _, m := range snap.Maintainers {
			email, name, username := m.Email, m.Name, m.Username
			ms = append(ms, gen.Maintainer{
				Email:    optionalNonEmpty(email),
				Name:     optionalNonEmpty(name),
				Username: optionalNonEmpty(username),
			})
		}
		out.Maintainers = &ms
	}
	if snap.LatestVersion != "" {
		v := snap.LatestVersion
		out.LatestVersion = &v
	}
	if snap.LatestVersionPublishedAt != nil {
		t := *snap.LatestVersionPublishedAt
		out.LatestVersionPublishedAt = &t
	}
	if snap.PublishMethod != "" {
		pm := gen.PublisherSnapshotPublishMethod(snap.PublishMethod)
		out.PublishMethod = &pm
	}
	if snap.SourceRepoURL != nil && *snap.SourceRepoURL != "" {
		v := *snap.SourceRepoURL
		out.SourceRepoURL = &v
	}
	return out
}

func optionalNonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
