package domain

import (
	"errors"
	"fmt"
	"time"
)

type RemediationKind string

const (
	RemediationPinVersion   RemediationKind = "pin_version"
	RemediationRotateSecret RemediationKind = "rotate_secret"
	RemediationOpenPR       RemediationKind = "open_pr"
	RemediationNotify       RemediationKind = "notify"
	RemediationDismiss      RemediationKind = "dismiss"
)

// Remediation is an append-only audit entry for an action taken on an Incident.
type Remediation struct {
	ID         string
	IncidentID string
	Kind       RemediationKind
	ExecutedAt time.Time
	ActorRef   string
	Details    map[string]any
}

var ErrInvalidRemediationKind = errors.New("invalid remediation kind")

var validRemediationKinds = map[RemediationKind]struct{}{
	RemediationPinVersion:   {},
	RemediationRotateSecret: {},
	RemediationOpenPR:       {},
	RemediationNotify:       {},
	RemediationDismiss:      {},
}

// Validate checks the Remediation's kind against the allowed set.
func (r Remediation) Validate() error {
	if _, ok := validRemediationKinds[r.Kind]; !ok {
		return fmt.Errorf("%w: %q", ErrInvalidRemediationKind, r.Kind)
	}
	return nil
}
