package domain_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

func TestIoCValidate(t *testing.T) {
	tests := []struct {
		name    string
		ioc     domain.IoC
		wantErr bool
	}{
		{
			"valid packageVersion",
			domain.IoC{
				Kind:           domain.IoCKindPackageVersion,
				PackageVersion: &domain.IoCPackageVersion{Name: "axios", Version: "1.11.0"},
			},
			false,
		},
		{
			"valid packageRange",
			domain.IoC{
				Kind:         domain.IoCKindPackageRange,
				PackageRange: &domain.IoCPackageRange{Name: "axios", Constraint: ">=1.10.0 <1.12.0"},
			},
			false,
		},
		{
			"valid publisherAnomaly",
			domain.IoC{
				Kind:             domain.IoCKindPublisherAnomaly,
				PublisherAnomaly: &domain.IoCPublisherAnomaly{PublisherName: "attacker"},
			},
			false,
		},
		{"no body", domain.IoC{Kind: domain.IoCKindPackageVersion}, true},
		{
			"two bodies",
			domain.IoC{
				Kind:           domain.IoCKindPackageVersion,
				PackageVersion: &domain.IoCPackageVersion{Name: "x"},
				PackageRange:   &domain.IoCPackageRange{Name: "x"},
			},
			true,
		},
		{
			"kind/body mismatch",
			domain.IoC{
				Kind:         domain.IoCKindPackageVersion,
				PackageRange: &domain.IoCPackageRange{Name: "x"},
			},
			true,
		},
		{
			"unknown kind",
			domain.IoC{
				Kind:           "sideChannel",
				PackageVersion: &domain.IoCPackageVersion{Name: "x"},
			},
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ioc.Validate()
			if tc.wantErr {
				require.Error(t, err)
				require.True(t, errors.Is(err, domain.ErrInvalidIoC))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSeverity_Values(t *testing.T) {
	// The strings are persisted; fix them in.
	require.Equal(t, domain.Severity("low"), domain.SeverityLow)
	require.Equal(t, domain.Severity("medium"), domain.SeverityMedium)
	require.Equal(t, domain.Severity("high"), domain.SeverityHigh)
	require.Equal(t, domain.Severity("critical"), domain.SeverityCritical)
}
