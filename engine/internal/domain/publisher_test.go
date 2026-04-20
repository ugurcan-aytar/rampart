package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

func TestSignalType_Values(t *testing.T) {
	// These strings are persisted and exposed over the API; changing a value
	// is a breaking change. Lock them in.
	require.Equal(t, domain.SignalType("new_maintainer_email"), domain.SignalNewMaintainerEmail)
	require.Equal(t, domain.SignalType("dormant_account_active"), domain.SignalDormantAccountActive)
	require.Equal(t, domain.SignalType("missing_git_tag"), domain.SignalMissingGitTag)
	require.Equal(t, domain.SignalType("off_hours_publish"), domain.SignalOffHoursPublish)
	require.Equal(t, domain.SignalType("oidc_regression"), domain.SignalOIDCRegression)
	require.Equal(t, domain.SignalType("version_jump"), domain.SignalVersionJump)
	require.Equal(t, domain.SignalType("low_download_day_attack"), domain.SignalLowDownloadDayAttack)
}

func TestPublisherProfile_Construction(t *testing.T) {
	now := time.Now().UTC()
	p := domain.PublisherProfile{
		Publisher: domain.Publisher{
			Ecosystem: "npm", Name: "axios-maintainer",
			FirstSeen: now.Add(-365 * 24 * time.Hour), LastSeen: now,
		},
		PackageCount:       12,
		PublishCount:       143,
		Last30DayPublishes: 2,
		UsesOIDC:           false,
		HasGitTags:         true,
		MaintainerEmails:   []string{"dev@example.com"},
	}
	require.Equal(t, "axios-maintainer", p.Publisher.Name)
	require.Equal(t, 12, p.PackageCount)
	require.False(t, p.UsesOIDC)
	require.True(t, p.HasGitTags)
}

func TestPublisherSignal_Construction(t *testing.T) {
	now := time.Now().UTC()
	sig := domain.PublisherSignal{
		Type:        domain.SignalNewMaintainerEmail,
		Publisher:   domain.Publisher{Ecosystem: "npm", Name: "x"},
		Severity:    domain.SeverityHigh,
		Description: "new email registered 3h before publish",
		Evidence:    map[string]any{"email_age_hours": 3},
		DetectedAt:  now,
	}
	require.Equal(t, domain.SignalNewMaintainerEmail, sig.Type)
	require.Equal(t, domain.SeverityHigh, sig.Severity)
	require.Equal(t, 3, sig.Evidence["email_age_hours"])
}
