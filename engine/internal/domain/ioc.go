package domain

import (
	"errors"
	"fmt"
	"time"
)

type IoCKind string

const (
	IoCKindPackageVersion   IoCKind = "packageVersion"
	IoCKindPackageRange     IoCKind = "packageRange"
	IoCKindPublisherAnomaly IoCKind = "publisherAnomaly"
)

type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// IoC is an Indicator of Compromise. Tagged union: exactly one of the body
// pointers must match Kind. Two body slots share the publisherAnomaly
// kind — `PublisherAnomaly` (maintainer-keyed, Theme D legacy) and
// `AnomalyBody` (package-keyed, Theme F2 detectors). See ADR-0014.
type IoC struct {
	ID          string
	Kind        IoCKind
	Severity    Severity
	Ecosystem   string
	Source      string
	PublishedAt time.Time
	Description string

	PackageVersion   *IoCPackageVersion
	PackageRange     *IoCPackageRange
	PublisherAnomaly *IoCPublisherAnomaly
	AnomalyBody      *IoCBodyAnomaly
}

type IoCPackageVersion struct {
	Name    string
	Version string
	PURL    string
}

type IoCPackageRange struct {
	Name       string
	Constraint string
}

type IoCPublisherAnomaly struct {
	PublisherName string
	Signals       []PublisherSignal
}

// IoCBodyAnomaly is the package-keyed publisher anomaly variant emitted
// by the Theme F2 anomaly orchestrator. PackageRef is the Theme F1
// `<ecosystem>:<name>` convention. See ADR-0014 for the bridge design.
type IoCBodyAnomaly struct {
	Kind        AnomalyKind
	Confidence  ConfidenceLevel
	Explanation string
	PackageRef  string
	Evidence    map[string]any
}

var ErrInvalidIoC = errors.New("invalid ioc")

// Validate enforces the tagged-union invariant and kind/body agreement.
// publisherAnomaly accepts EITHER PublisherAnomaly (legacy maintainer-
// keyed) OR AnomalyBody (Theme F2 package-keyed) — exactly one must be
// set when Kind is publisherAnomaly. See ADR-0014.
func (i IoC) Validate() error {
	set := 0
	if i.PackageVersion != nil {
		set++
	}
	if i.PackageRange != nil {
		set++
	}
	if i.PublisherAnomaly != nil {
		set++
	}
	if i.AnomalyBody != nil {
		set++
	}
	if set != 1 {
		return fmt.Errorf("%w: exactly one body must be set, got %d", ErrInvalidIoC, set)
	}
	switch i.Kind {
	case IoCKindPackageVersion:
		if i.PackageVersion == nil {
			return fmt.Errorf("%w: kind=packageVersion but PackageVersion body is nil", ErrInvalidIoC)
		}
	case IoCKindPackageRange:
		if i.PackageRange == nil {
			return fmt.Errorf("%w: kind=packageRange but PackageRange body is nil", ErrInvalidIoC)
		}
	case IoCKindPublisherAnomaly:
		if i.PublisherAnomaly == nil && i.AnomalyBody == nil {
			return fmt.Errorf("%w: kind=publisherAnomaly but neither PublisherAnomaly nor AnomalyBody is set", ErrInvalidIoC)
		}
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidIoC, i.Kind)
	}
	return nil
}
