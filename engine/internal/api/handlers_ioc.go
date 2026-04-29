package api

import (
	"net/http"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// SubmitIoC publishes a new Indicator of Compromise and forward-matches
// it against every stored SBOM. Each newly-matched component (dedup'd)
// opens one incident and the bus sees one `ioc.matched` + one
// `incident.opened` per affected component. Idempotent by (IoC ID,
// ComponentRef): re-publishing the same IoC against the same component
// while its incident is open is a no-op.
func (s *Server) SubmitIoC(w http.ResponseWriter, r *http.Request) {
	var body gen.IoC
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
		return
	}
	ioc := fromGenIoC(body)
	if err := ioc.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_IOC", err.Error())
		return
	}
	// Constraint strings are authored by operators and feeds; reject a
	// bad one at publish time instead of silently never-matching.
	if ioc.Kind == domain.IoCKindPackageRange && ioc.PackageRange != nil {
		if _, err := semver.NewConstraint(ioc.PackageRange.Constraint); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_CONSTRAINT", err.Error())
			return
		}
	}

	if err := s.storage.UpsertIoC(r.Context(), ioc); err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	pairs, matchedComponents, err := s.forwardMatch(r.Context(), ioc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "MATCH_FAILED", err.Error())
		return
	}
	if _, err := s.openIncidentsForMatches(r.Context(), pairs); err != nil {
		writeError(w, http.StatusInternalServerError, "INCIDENT_OPEN_FAILED", err.Error())
		return
	}
	if len(matchedComponents) > 0 {
		s.events.Publish(domain.IoCMatchedEvent{
			IoCID:             ioc.ID,
			MatchedComponents: matchedComponents,
			At:                time.Now().UTC(),
		})
	}

	writeJSON(w, http.StatusCreated, toGenIoC(ioc))
}

// ListIoCs returns all IoCs sorted by PublishedAt. Pagination is
// deferred (no specific theme yet); the Cursor / Limit params are
// accepted but ignored.
func (s *Server) ListIoCs(w http.ResponseWriter, r *http.Request, params gen.ListIoCsParams) {
	iocs, err := s.storage.ListIoCs(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}
	filtered := iocs[:0]
	for _, i := range iocs {
		if params.Ecosystem != nil && *params.Ecosystem != "" && i.Ecosystem != *params.Ecosystem {
			continue
		}
		if params.Severity != nil && domain.Severity(*params.Severity) != i.Severity {
			continue
		}
		filtered = append(filtered, i)
	}
	items := make([]gen.IoC, 0, len(filtered))
	for _, i := range filtered {
		items = append(items, toGenIoC(i))
	}
	writeJSON(w, http.StatusOK, gen.IoCPage{Items: items})
}

func fromGenIoC(g gen.IoC) domain.IoC {
	i := domain.IoC{
		ID:          g.Id,
		Kind:        domain.IoCKind(g.Kind),
		Severity:    domain.Severity(g.Severity),
		Ecosystem:   g.Ecosystem,
		PublishedAt: g.PublishedAt,
	}
	if g.Source != nil {
		i.Source = *g.Source
	}
	if g.Description != nil {
		i.Description = *g.Description
	}
	if g.PackageVersion != nil {
		i.PackageVersion = &domain.IoCPackageVersion{
			Name:    g.PackageVersion.Name,
			Version: g.PackageVersion.Version,
			PURL:    g.PackageVersion.Purl,
		}
	}
	if g.PackageRange != nil {
		i.PackageRange = &domain.IoCPackageRange{
			Name:       g.PackageRange.Name,
			Constraint: g.PackageRange.Constraint,
		}
	}
	if g.PublisherAnomaly != nil {
		// PublisherAnomaly IoCKind is intentionally a no-op slot in
		// forwardMatch dispatch. Per ADR-0014, anomaly-derived IoCs
		// surface via the IoCBodyAnomaly variant of IoCBody, not as
		// a separate kind. The slot remains for backwards compatibility
		// and potential future use.
		i.PublisherAnomaly = &domain.IoCPublisherAnomaly{
			PublisherName: g.PublisherAnomaly.PublisherName,
		}
	}
	return i
}

func toGenIoC(i domain.IoC) gen.IoC {
	out := gen.IoC{
		Id:          i.ID,
		Kind:        gen.IoCKind(i.Kind),
		Severity:    gen.Severity(i.Severity),
		Ecosystem:   i.Ecosystem,
		PublishedAt: i.PublishedAt,
	}
	if i.Source != "" {
		s := i.Source
		out.Source = &s
	}
	if i.Description != "" {
		d := i.Description
		out.Description = &d
	}
	if i.PackageVersion != nil {
		out.PackageVersion = &gen.IoCPackageVersion{
			Name:    i.PackageVersion.Name,
			Version: i.PackageVersion.Version,
			Purl:    i.PackageVersion.PURL,
		}
	}
	if i.PackageRange != nil {
		out.PackageRange = &gen.IoCPackageRange{
			Name:       i.PackageRange.Name,
			Constraint: i.PackageRange.Constraint,
		}
	}
	if i.PublisherAnomaly != nil {
		out.PublisherAnomaly = &gen.IoCPublisherAnomaly{
			PublisherName: i.PublisherAnomaly.PublisherName,
		}
	}
	return out
}
