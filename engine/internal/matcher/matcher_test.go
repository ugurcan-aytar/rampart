package matcher_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/matcher"
)

func sbomWith(pkgs ...domain.PackageVersion) domain.SBOM {
	return domain.SBOM{Ecosystem: "npm", Packages: pkgs}
}

func pkg(name, version string) domain.PackageVersion {
	return domain.PackageVersion{
		Ecosystem: "npm",
		Name:      name,
		Version:   version,
		PURL:      domain.CanonicalPURL("npm", name, version),
	}
}

func TestEvaluate_PackageVersion_Hit(t *testing.T) {
	ioc := domain.IoC{
		Kind:      domain.IoCKindPackageVersion,
		Ecosystem: "npm",
		PackageVersion: &domain.IoCPackageVersion{
			Name:    "axios",
			Version: "1.11.0",
		},
	}
	sbom := sbomWith(pkg("lodash", "4.17.21"), pkg("axios", "1.11.0"))

	m := matcher.Evaluate(ioc, sbom)

	require.True(t, m.Matched)
	require.Len(t, m.Packages, 1)
	require.Equal(t, "axios", m.Packages[0].Name)
	require.Equal(t, "1.11.0", m.Packages[0].Version)
}

func TestEvaluate_PackageVersion_NoHit(t *testing.T) {
	ioc := domain.IoC{
		Kind:      domain.IoCKindPackageVersion,
		Ecosystem: "npm",
		PackageVersion: &domain.IoCPackageVersion{
			Name:    "axios",
			Version: "1.11.0",
		},
	}
	sbom := sbomWith(pkg("axios", "1.10.5"), pkg("react", "18.2.0"))

	m := matcher.Evaluate(ioc, sbom)

	require.False(t, m.Matched)
	require.Empty(t, m.Packages)
}

func TestEvaluate_PackageRange_Hit(t *testing.T) {
	ioc := domain.IoC{
		Kind:      domain.IoCKindPackageRange,
		Ecosystem: "npm",
		PackageRange: &domain.IoCPackageRange{
			Name:       "axios",
			Constraint: ">=1.11.0, <1.12.0",
		},
	}
	sbom := sbomWith(pkg("axios", "1.11.3"), pkg("lodash", "4.17.21"))

	m := matcher.Evaluate(ioc, sbom)

	require.True(t, m.Matched)
	require.Len(t, m.Packages, 1)
	require.Equal(t, "axios", m.Packages[0].Name)
}

func TestEvaluate_PackageRange_OutOfRange(t *testing.T) {
	ioc := domain.IoC{
		Kind:      domain.IoCKindPackageRange,
		Ecosystem: "npm",
		PackageRange: &domain.IoCPackageRange{
			Name:       "axios",
			Constraint: ">=1.11.0",
		},
	}
	sbom := sbomWith(pkg("axios", "1.10.5"))

	m := matcher.Evaluate(ioc, sbom)

	require.False(t, m.Matched)
}

func TestEvaluate_PackageRange_InvalidConstraint(t *testing.T) {
	// A broken constraint stored on an IoC must not crash Evaluate — the
	// engine keeps ingesting, the matcher returns no hits. (Bad input is
	// the business of SubmitIoC validation, not this package.)
	ioc := domain.IoC{
		Kind:      domain.IoCKindPackageRange,
		Ecosystem: "npm",
		PackageRange: &domain.IoCPackageRange{
			Name:       "axios",
			Constraint: "not-a-semver-constraint",
		},
	}
	m := matcher.Evaluate(ioc, sbomWith(pkg("axios", "1.11.0")))
	require.False(t, m.Matched)
}

func TestEvaluate_PackageRange_NonSemverPackageVersion(t *testing.T) {
	// npm lockfiles sometimes carry non-semver "versions" like
	// git+https://… references. These can never be compared to a
	// semver constraint and must be skipped silently.
	ioc := domain.IoC{
		Kind:      domain.IoCKindPackageRange,
		Ecosystem: "npm",
		PackageRange: &domain.IoCPackageRange{
			Name:       "some-pkg",
			Constraint: ">=1.0.0",
		},
	}
	sbom := sbomWith(domain.PackageVersion{
		Ecosystem: "npm",
		Name:      "some-pkg",
		Version:   "git+https://example.com/repo.git#abc",
	})
	m := matcher.Evaluate(ioc, sbom)
	require.False(t, m.Matched)
}

func TestEvaluate_PublisherAnomaly_IsNoOpInPhase1(t *testing.T) {
	ioc := domain.IoC{
		Kind:      domain.IoCKindPublisherAnomaly,
		Ecosystem: "npm",
		PublisherAnomaly: &domain.IoCPublisherAnomaly{
			PublisherName: "some-maintainer",
		},
	}
	m := matcher.Evaluate(ioc, sbomWith(pkg("axios", "1.11.0")))
	require.False(t, m.Matched, "Phase 1: publisherAnomaly IoCs must never match")
}

func TestEvaluate_EcosystemMismatch_NoMatch(t *testing.T) {
	// An npm IoC vs. a pypi SBOM is a no-match by construction — guards
	// the engine against operator error when the same package name
	// exists across ecosystems (e.g. pypi `requests` vs. npm `requests`).
	ioc := domain.IoC{
		Kind:      domain.IoCKindPackageVersion,
		Ecosystem: "npm",
		PackageVersion: &domain.IoCPackageVersion{
			Name:    "axios",
			Version: "1.11.0",
		},
	}
	sbom := domain.SBOM{Ecosystem: "pypi", Packages: []domain.PackageVersion{pkg("axios", "1.11.0")}}
	m := matcher.Evaluate(ioc, sbom)
	require.False(t, m.Matched)
}

func TestEvaluate_MultipleHitsInOneSBOM(t *testing.T) {
	// Monorepo vendoring: the same package pinned at two versions under
	// different node_modules paths. A range IoC covering both must
	// return both hits.
	ioc := domain.IoC{
		Kind:      domain.IoCKindPackageRange,
		Ecosystem: "npm",
		PackageRange: &domain.IoCPackageRange{
			Name:       "axios",
			Constraint: ">=1.11.0, <1.12.0",
		},
	}
	sbom := sbomWith(
		pkg("axios", "1.11.0"),
		pkg("axios", "1.11.3"),
		pkg("lodash", "4.17.21"),
	)
	m := matcher.Evaluate(ioc, sbom)
	require.True(t, m.Matched)
	require.Len(t, m.Packages, 2)
}
