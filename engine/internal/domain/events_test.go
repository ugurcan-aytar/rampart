package domain_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

func TestDomainEvents_Interface(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name      string
		event     domain.DomainEvent
		wantType  string
		wantAggID string
	}{
		{"opened", domain.IncidentOpenedEvent{IncidentID: "INC1", IoCID: "IOC1", At: now}, "incident.opened", "INC1"},
		{"transitioned", domain.IncidentTransitionedEvent{IncidentID: "INC1", From: domain.StatePending, To: domain.StateTriaged, At: now}, "incident.transitioned", "INC1"},
		{"remediation", domain.RemediationAddedEvent{IncidentID: "INC1", RemediationID: "REM1", Kind: domain.RemediationPinVersion, At: now}, "remediation.added", "INC1"},
		{"sbom", domain.SBOMIngestedEvent{SBOMID: "SBOM1", ComponentRef: "kind:Component/default/web", At: now}, "sbom.ingested", "SBOM1"},
		{"ioc", domain.IoCMatchedEvent{IoCID: "IOC1", MatchedComponents: []string{"kind:Component/default/web"}, At: now}, "ioc.matched", "IOC1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.wantType, tc.event.EventType())
			require.Equal(t, tc.wantAggID, tc.event.AggregateID())
			require.WithinDuration(t, now, tc.event.OccurredAt(), 0)
		})
	}
}
