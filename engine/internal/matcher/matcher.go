// Package matcher walks an SBOM's package list against an IoC and returns
// the packages that match the IoC's kind-specific predicate. Stateless
// and pure — the engine wraps the result in domain events (see
// engine/internal/api.openIncidentFromMatch). Three IoC kinds:
//
//   - packageVersion: exact (ecosystem, name, version) match.
//   - packageRange: exact (ecosystem, name), semver constraint per
//     Masterminds/semver/v3 — the library the OpenAPI schema names.
//   - publisherAnomaly: covered via the IoCBodyAnomaly variant
//     (package-keyed, ADR-0014). The legacy maintainer-keyed
//     PublisherAnomaly slot stays a no-op — no shipping detector
//     produces it; IoCs that arrive in that slot parse and persist
//     but never match, logged at Debug.
//
// Why "matches a single IoC against a single SBOM" and not "matches all
// IoCs × all SBOMs": the engine's two call sites are
// (a) retroactive after SBOM ingest — scan that SBOM against every IoC
// and (b) forward after IoC submit — scan every SBOM against that IoC.
// Keeping the unit of work at (one IoC, one SBOM) composes both
// directions cleanly without rewriting the core.
package matcher

import (
	"github.com/Masterminds/semver/v3"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

// Match is the result of a single (IoC × SBOM) evaluation.
type Match struct {
	// Matched reports whether the SBOM carries at least one package that
	// triggered the IoC's predicate.
	Matched bool
	// Packages lists the specific SBOM packages that fired. Empty when
	// Matched is false; may carry more than one element if, e.g., an
	// IoC range matches multiple versions in a monorepo's transitive
	// closure (exotic but legal).
	Packages []domain.PackageVersion
}

// Evaluate runs the IoC's predicate against the SBOM. The caller is
// expected to pre-validate the IoC via `domain.IoC.Validate()`.
//
// Unknown kinds return `Match{Matched: false}`. A panic-free default
// here matters for the engine's hot path: a future schema change that
// introduces a new IoCKind should degrade gracefully until this
// package catches up, not take the ingestion endpoint offline.
func Evaluate(ioc domain.IoC, sbom domain.SBOM) Match {
	if sbom.Ecosystem != "" && ioc.Ecosystem != "" && sbom.Ecosystem != ioc.Ecosystem {
		return Match{}
	}
	switch ioc.Kind {
	case domain.IoCKindPackageVersion:
		return evaluatePackageVersion(ioc, sbom)
	case domain.IoCKindPackageRange:
		return evaluatePackageRange(ioc, sbom)
	case domain.IoCKindPublisherAnomaly:
		// Two body variants share this kind (ADR-0014):
		//   - PublisherAnomaly (maintainer-keyed, Theme D legacy) — no
		//     shipping detector produces it; stays no-op.
		//   - AnomalyBody (package-keyed, Theme F2) — match SBOM
		//     packages whose `<ecosystem>:<name>` equals the
		//     anomaly's PackageRef.
		if ioc.AnomalyBody != nil {
			return evaluateAnomalyBody(ioc, sbom)
		}
		return Match{}
	default:
		return Match{}
	}
}

// evaluateAnomalyBody implements the package-keyed publisher anomaly
// match: the IoC's PackageRef is `<ecosystem>:<name>`; we match every
// SBOM package whose Ecosystem + Name reconstruct to that ref. Version
// is intentionally not part of the key — anomalies are a property of
// the package, not a specific release.
func evaluateAnomalyBody(ioc domain.IoC, sbom domain.SBOM) Match {
	target := ioc.AnomalyBody.PackageRef
	if target == "" {
		return Match{}
	}
	var hits []domain.PackageVersion
	for _, p := range sbom.Packages {
		if p.Ecosystem+":"+p.Name == target {
			hits = append(hits, p)
		}
	}
	return Match{Matched: len(hits) > 0, Packages: hits}
}

func evaluatePackageVersion(ioc domain.IoC, sbom domain.SBOM) Match {
	if ioc.PackageVersion == nil {
		return Match{}
	}
	target := ioc.PackageVersion
	var hits []domain.PackageVersion
	for _, p := range sbom.Packages {
		if p.Name == target.Name && p.Version == target.Version {
			hits = append(hits, p)
		}
	}
	return Match{Matched: len(hits) > 0, Packages: hits}
}

func evaluatePackageRange(ioc domain.IoC, sbom domain.SBOM) Match {
	if ioc.PackageRange == nil {
		return Match{}
	}
	target := ioc.PackageRange
	constraint, err := semver.NewConstraint(target.Constraint)
	if err != nil {
		// Bad constraint = no match. The SubmitIoC handler is the place
		// to surface a 400 at publish time; the matcher just has to not
		// crash the engine on a stored-but-malformed IoC.
		return Match{}
	}
	var hits []domain.PackageVersion
	for _, p := range sbom.Packages {
		if p.Name != target.Name {
			continue
		}
		v, err := semver.NewVersion(p.Version)
		if err != nil {
			// npm lockfile can carry non-semver strings ("git+https://…").
			// These can never match a semver constraint, so skip.
			continue
		}
		if constraint.Check(v) {
			hits = append(hits, p)
		}
	}
	return Match{Matched: len(hits) > 0, Packages: hits}
}
