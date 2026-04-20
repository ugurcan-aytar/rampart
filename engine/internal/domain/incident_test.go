package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

func TestIncidentState_CanTransitionTo(t *testing.T) {
	allowed := map[domain.IncidentState][]domain.IncidentState{
		domain.StatePending:      {domain.StateTriaged, domain.StateDismissed},
		domain.StateTriaged:      {domain.StateAcknowledged, domain.StateDismissed},
		domain.StateAcknowledged: {domain.StateRemediating, domain.StateDismissed},
		domain.StateRemediating:  {domain.StateClosed},
	}
	all := []domain.IncidentState{
		domain.StatePending, domain.StateTriaged, domain.StateAcknowledged,
		domain.StateRemediating, domain.StateClosed, domain.StateDismissed,
	}
	for _, from := range all {
		for _, to := range all {
			want := from == to // idempotent
			for _, a := range allowed[from] {
				if a == to {
					want = true
				}
			}
			if got := from.CanTransitionTo(to); got != want {
				t.Errorf("%s → %s: got %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestIncident_Transition(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		now := time.Now().UTC()
		inc := &domain.Incident{State: domain.StatePending}
		require.NoError(t, inc.Transition(domain.StateTriaged, now))
		require.Equal(t, domain.StateTriaged, inc.State)
		require.Equal(t, now, inc.LastTransitionedAt)
	})
	t.Run("idempotent does not touch timestamp", func(t *testing.T) {
		original := time.Unix(100, 0)
		inc := &domain.Incident{State: domain.StatePending, LastTransitionedAt: original}
		require.NoError(t, inc.Transition(domain.StatePending, time.Unix(200, 0)))
		require.Equal(t, original, inc.LastTransitionedAt)
	})
	t.Run("illegal transition does not mutate state", func(t *testing.T) {
		inc := &domain.Incident{State: domain.StatePending}
		err := inc.Transition(domain.StateClosed, time.Now())
		require.Error(t, err)
		require.True(t, errors.Is(err, domain.ErrInvalidTransition))
		require.Equal(t, domain.StatePending, inc.State)
	})
	t.Run("unknown state", func(t *testing.T) {
		inc := &domain.Incident{State: "bogus"}
		err := inc.Transition(domain.StateTriaged, time.Now())
		require.Error(t, err)
		require.True(t, errors.Is(err, domain.ErrUnknownState))
	})
	t.Run("terminal closed rejects all", func(t *testing.T) {
		inc := &domain.Incident{State: domain.StateClosed}
		for _, next := range []domain.IncidentState{
			domain.StatePending, domain.StateTriaged, domain.StateAcknowledged,
			domain.StateRemediating, domain.StateDismissed,
		} {
			err := inc.Transition(next, time.Now())
			require.Error(t, err, "closed → %s must be rejected", next)
			require.True(t, errors.Is(err, domain.ErrInvalidTransition))
		}
	})
	t.Run("terminal dismissed rejects all", func(t *testing.T) {
		inc := &domain.Incident{State: domain.StateDismissed}
		err := inc.Transition(domain.StateRemediating, time.Now())
		require.Error(t, err)
	})
}
