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

// IoC is an Indicator of Compromise. Tagged union: exactly one of the three
// sub-struct pointers must match Kind.
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

var ErrInvalidIoC = errors.New("invalid ioc")

// Validate enforces the tagged-union invariant and kind/body agreement.
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
		if i.PublisherAnomaly == nil {
			return fmt.Errorf("%w: kind=publisherAnomaly but PublisherAnomaly body is nil", ErrInvalidIoC)
		}
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidIoC, i.Kind)
	}
	return nil
}
