package domain_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

func TestRemediation_Validate(t *testing.T) {
	valid := []domain.RemediationKind{
		domain.RemediationPinVersion,
		domain.RemediationRotateSecret,
		domain.RemediationOpenPR,
		domain.RemediationNotify,
		domain.RemediationDismiss,
	}
	for _, k := range valid {
		t.Run(string(k), func(t *testing.T) {
			r := domain.Remediation{Kind: k}
			require.NoError(t, r.Validate())
		})
	}

	t.Run("invalid kind", func(t *testing.T) {
		r := domain.Remediation{Kind: "delete_all_the_things"}
		err := r.Validate()
		require.Error(t, err)
		require.True(t, errors.Is(err, domain.ErrInvalidRemediationKind))
	})

	t.Run("empty kind", func(t *testing.T) {
		r := domain.Remediation{}
		err := r.Validate()
		require.Error(t, err)
	})
}

func TestRemediationKind_Strings(t *testing.T) {
	require.Equal(t, domain.RemediationKind("pin_version"), domain.RemediationPinVersion)
	require.Equal(t, domain.RemediationKind("rotate_secret"), domain.RemediationRotateSecret)
	require.Equal(t, domain.RemediationKind("open_pr"), domain.RemediationOpenPR)
	require.Equal(t, domain.RemediationKind("notify"), domain.RemediationNotify)
	require.Equal(t, domain.RemediationKind("dismiss"), domain.RemediationDismiss)
}
