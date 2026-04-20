package domain

import "time"

// DomainEvent is the contract every event-sourced state change satisfies.
// Each concrete event type carries its own fields plus At timestamp.
type DomainEvent interface {
	EventType() string
	AggregateID() string
	OccurredAt() time.Time
}

type IncidentOpenedEvent struct {
	IncidentID                 string
	IoCID                      string
	AffectedComponentsSnapshot []string
	At                         time.Time
}

func (IncidentOpenedEvent) EventType() string       { return "incident.opened" }
func (e IncidentOpenedEvent) AggregateID() string   { return e.IncidentID }
func (e IncidentOpenedEvent) OccurredAt() time.Time { return e.At }

type IncidentTransitionedEvent struct {
	IncidentID string
	From       IncidentState
	To         IncidentState
	At         time.Time
}

func (IncidentTransitionedEvent) EventType() string       { return "incident.transitioned" }
func (e IncidentTransitionedEvent) AggregateID() string   { return e.IncidentID }
func (e IncidentTransitionedEvent) OccurredAt() time.Time { return e.At }

type RemediationAddedEvent struct {
	IncidentID    string
	RemediationID string
	Kind          RemediationKind
	At            time.Time
}

func (RemediationAddedEvent) EventType() string       { return "remediation.added" }
func (e RemediationAddedEvent) AggregateID() string   { return e.IncidentID }
func (e RemediationAddedEvent) OccurredAt() time.Time { return e.At }

type SBOMIngestedEvent struct {
	SBOMID       string
	ComponentRef string
	At           time.Time
}

func (SBOMIngestedEvent) EventType() string       { return "sbom.ingested" }
func (e SBOMIngestedEvent) AggregateID() string   { return e.SBOMID }
func (e SBOMIngestedEvent) OccurredAt() time.Time { return e.At }

type IoCMatchedEvent struct {
	IoCID             string
	MatchedComponents []string
	At                time.Time
}

func (IoCMatchedEvent) EventType() string       { return "ioc.matched" }
func (e IoCMatchedEvent) AggregateID() string   { return e.IoCID }
func (e IoCMatchedEvent) OccurredAt() time.Time { return e.At }
