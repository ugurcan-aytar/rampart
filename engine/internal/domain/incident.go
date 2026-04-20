package domain

import (
	"errors"
	"fmt"
	"time"
)

type IncidentState string

const (
	StatePending      IncidentState = "pending"
	StateTriaged      IncidentState = "triaged"
	StateAcknowledged IncidentState = "acknowledged"
	StateRemediating  IncidentState = "remediating"
	StateClosed       IncidentState = "closed"
	StateDismissed    IncidentState = "dismissed"
)

// Incident pins the blast radius at open time: once AffectedComponentsSnapshot
// is set, subsequent SBOM changes do not alter it.
type Incident struct {
	ID                         string
	IoCID                      string
	State                      IncidentState
	OpenedAt                   time.Time
	LastTransitionedAt         time.Time
	AffectedComponentsSnapshot []string
	Remediations               []Remediation
}

var (
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrUnknownState      = errors.New("unknown incident state")
)

var allowedTransitions = map[IncidentState][]IncidentState{
	StatePending:      {StateTriaged, StateDismissed},
	StateTriaged:      {StateAcknowledged, StateDismissed},
	StateAcknowledged: {StateRemediating, StateDismissed},
	StateRemediating:  {StateClosed},
	StateClosed:       {},
	StateDismissed:    {},
}

// CanTransitionTo reports whether the receiver state may advance to next.
// Self-transitions are always legal (idempotency).
func (s IncidentState) CanTransitionTo(next IncidentState) bool {
	if s == next {
		return true
	}
	allowed, ok := allowedTransitions[s]
	if !ok {
		return false
	}
	for _, t := range allowed {
		if t == next {
			return true
		}
	}
	return false
}

// Transition moves the incident's state and refreshes LastTransitionedAt.
// Idempotent: Transition(same, _) is a no-op that does not touch timestamps.
func (i *Incident) Transition(next IncidentState, now time.Time) error {
	if _, ok := allowedTransitions[i.State]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownState, i.State)
	}
	if !i.State.CanTransitionTo(next) {
		return fmt.Errorf("%w: %s → %s", ErrInvalidTransition, i.State, next)
	}
	if i.State == next {
		return nil
	}
	i.State = next
	i.LastTransitionedAt = now
	return nil
}
