package trust_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
	"github.com/ugurcan-aytar/rampart/engine/internal/trust"
)

func TestAlwaysTrust_Empty(t *testing.T) {
	signals, err := trust.AlwaysTrust{}.Evaluate(context.Background(), domain.PublisherProfile{})
	require.NoError(t, err)
	require.Empty(t, signals)
}

func TestAlwaysTrust_ReturnsNothing(t *testing.T) {
	// Feed a profile that a real detector would almost certainly flag:
	// a publisher that is not using OIDC, with a brand-new maintainer email,
	// a large install-time gap, and a suspicious domain. AlwaysTrust must
	// still return no signals — that is the whole point of the Phase 1 stub.
	recent := time.Now().Add(-2 * time.Hour)
	full := domain.PublisherProfile{
		Publisher: domain.Publisher{
			Ecosystem: "npm", Name: "axios-maintainer",
			FirstSeen: time.Now().Add(-365 * 24 * time.Hour),
			LastSeen:  time.Now(),
		},
		PackageCount:       42,
		PublishCount:       900,
		Last30DayPublishes: 3,
		UsesOIDC:           false,
		HasGitTags:         false,
		MaintainerEmails:   []string{"legit@example.com", "attacker@ru-domain.ru"},
		LastEmailChange:    &recent,
	}
	signals, err := trust.AlwaysTrust{}.Evaluate(context.Background(), full)
	require.NoError(t, err)
	require.Empty(t, signals, "Phase 1 AlwaysTrust must raise nothing — swap implementation in Phase 3")
}

func TestEngineInterface_Compliance(t *testing.T) {
	var e trust.Engine = trust.AlwaysTrust{}
	require.NotNil(t, e)

	// Exercise the interface — a reminder that Engine.Evaluate is the contract
	// future implementations replace, not AlwaysTrust directly.
	signals, err := e.Evaluate(context.Background(), domain.PublisherProfile{})
	require.NoError(t, err)
	require.Empty(t, signals)
}
