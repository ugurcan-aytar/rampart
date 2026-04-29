package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/api/gen"
	"github.com/ugurcan-aytar/rampart/engine/ingestion"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/storage"
)

// SubmitSBOM parses a lockfile submitted for a component, stores the
// resulting SBOM, and retroactively matches it against every stored
// IoC. Each match opens an incident (idempotent per (IoC, ComponentRef))
// and publishes `incident.opened` + `ioc.matched` on the event bus.
func (s *Server) SubmitSBOM(w http.ResponseWriter, r *http.Request, componentRef gen.ComponentRef) {
	// The component must be registered first — otherwise a typo in a
	// scaffolder task would create phantom SBOMs belonging to nothing.
	if _, err := s.storage.GetComponent(r.Context(), componentRef); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "COMPONENT_NOT_FOUND",
				"component "+componentRef+" is not registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}

	var body gen.SBOMSubmission
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
		return
	}
	if body.Ecosystem != "npm" {
		writeError(w, http.StatusBadRequest, "UNSUPPORTED_ECOSYSTEM",
			"HTTP submission currently supports npm lockfiles only; got "+body.Ecosystem+
				" (the rampart CLI scan command supports npm, gomod, cargo, pypi, maven)")
		return
	}
	if body.SourceFormat != gen.SBOMSubmissionSourceFormatNpmPackageLockV3 {
		writeError(w, http.StatusBadRequest, "UNSUPPORTED_SOURCE_FORMAT",
			"HTTP submission currently supports npm-package-lock-v3 only")
		return
	}

	// oapi-codegen decodes `content` (openapi `format: byte`) straight
	// into []byte, so the base64 layer is already unwound here.
	parsed, err := s.parser.Parse(r.Context(), body.Content)
	if err != nil {
		writeError(w, http.StatusBadRequest, "PARSE_FAILED", err.Error())
		return
	}

	var commitSHA string
	if body.CommitSha != nil {
		commitSHA = *body.CommitSha
	}
	sbom := ingestion.Ingest(parsed, componentRef, commitSHA)

	if err := s.storage.UpsertSBOM(r.Context(), *sbom); err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}
	s.events.Publish(domain.SBOMIngestedEvent{
		SBOMID:       sbom.ID,
		ComponentRef: sbom.ComponentRef,
		At:           sbom.GeneratedAt,
	})

	pairs, matchedIoCs, err := s.retroactiveMatch(r.Context(), *sbom)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "MATCH_FAILED", err.Error())
		return
	}
	if _, err := s.openIncidentsForMatches(r.Context(), pairs); err != nil {
		writeError(w, http.StatusInternalServerError, "INCIDENT_OPEN_FAILED", err.Error())
		return
	}
	for _, iocID := range matchedIoCs {
		s.events.Publish(domain.IoCMatchedEvent{
			IoCID:             iocID,
			MatchedComponents: []string{sbom.ComponentRef},
			At:                time.Now().UTC(),
		})
	}

	writeJSON(w, http.StatusCreated, toGenSBOM(*sbom))
}

func (s *Server) ListSBOMsByComponent(w http.ResponseWriter, r *http.Request, componentRef gen.ComponentRef) {
	sboms, err := s.storage.ListSBOMsByComponent(r.Context(), componentRef)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}
	items := make([]gen.SBOM, 0, len(sboms))
	for _, b := range sboms {
		items = append(items, toGenSBOM(b))
	}
	// Reusing ComponentPage would be wrong — the spec carries an
	// SBOM-typed envelope; we write a shape-compatible ad-hoc response.
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) GetSBOM(w http.ResponseWriter, r *http.Request, id string) {
	b, err := s.storage.GetSBOM(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			writeError(w, http.StatusNotFound, "SBOM_NOT_FOUND", "sbom "+id+" not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "STORAGE_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toGenSBOM(*b))
}

func toGenSBOM(b domain.SBOM) gen.SBOM {
	out := gen.SBOM{
		Id:           b.ID,
		ComponentRef: b.ComponentRef,
		Ecosystem:    b.Ecosystem,
		GeneratedAt:  b.GeneratedAt,
		SourceFormat: gen.SBOMSourceFormat(b.SourceFormat),
	}
	if b.CommitSHA != "" {
		cs := b.CommitSHA
		out.CommitSha = &cs
	}
	if b.SourceBytes > 0 {
		sb := b.SourceBytes
		out.SourceBytes = &sb
	}
	if len(b.Packages) > 0 {
		pkgs := make([]gen.PackageVersion, len(b.Packages))
		for i, p := range b.Packages {
			pkgs[i] = toGenPackageVersion(p)
		}
		out.Packages = &pkgs
	}
	return out
}

func toGenPackageVersion(p domain.PackageVersion) gen.PackageVersion {
	out := gen.PackageVersion{
		Ecosystem: p.Ecosystem,
		Name:      p.Name,
		Version:   p.Version,
		Purl:      p.PURL,
	}
	if p.Integrity != "" {
		integ := p.Integrity
		out.Integrity = &integ
	}
	if len(p.Scope) > 0 {
		scopes := make([]gen.PackageVersionScope, len(p.Scope))
		for i, sc := range p.Scope {
			scopes[i] = gen.PackageVersionScope(sc)
		}
		out.Scope = &scopes
	}
	return out
}
