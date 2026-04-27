// Package maintainerdrift detects an account-takeover pattern: a
// package's maintainer-email set adds a never-seen-before address
// shortly before (or shortly after) a new version publish.
//
// Confidence model:
//   - High: new email + version published within 7 days + the email
//     does not appear anywhere in prior snapshot history.
//   - Medium: new email + version published 7-14 days ago.
//   - Low: new email but the latest version is older than 14 days
//     (drift happened, but no proximate publish to weaponise it).
//
// Edge cases handled:
//   - History of length < 2 → no baseline, returns no anomaly.
//   - Email comparison is case-insensitive (RFC 5321 local-part is
//     case-sensitive in theory, but registries normalise; matching
//     case-sensitively would over-fire).
//   - A maintainer being ADDED on top of the baseline counts the
//     same as one being CHANGED — both are new addresses with
//     publish access. Detectors don't distinguish "join the team"
//     from "took over the team" since registries don't expose roles.
package maintainerdrift

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ugurcan-aytar/rampart/engine/internal/domain"
)

const (
	highWindow   = 7 * 24 * time.Hour
	mediumWindow = 14 * 24 * time.Hour
)

// Detector implements [anomaly.Detector] for the email-drift class.
type Detector struct {
	// now is injected so tests can supply a deterministic clock.
	now func() time.Time
}

// New returns a Detector with the wall-clock as `now`.
func New() *Detector {
	return &Detector{now: func() time.Time { return time.Now().UTC() }}
}

// Kind reports the detector's anomaly class.
func (d *Detector) Kind() domain.AnomalyKind {
	return domain.AnomalyKindMaintainerEmailDrift
}

// Detect implements [anomaly.Detector].
func (d *Detector) Detect(_ context.Context, packageRef string, history []domain.PublisherSnapshot) ([]domain.Anomaly, error) {
	if len(history) < 2 {
		return nil, nil
	}
	// History is newest-first. We compare against the IMMEDIATELY-prior
	// snapshot — that's the "what changed since the last tick" view —
	// and additionally consult the union of every prior snapshot to
	// downgrade confidence for addresses that merely re-appeared after
	// dropping out (registry blip rather than a real takeover).
	newest := history[0]
	previous := history[1]

	prior := emailSet(previous.Maintainers)
	current := emailSet(newest.Maintainers)

	everSeen := make(map[string]struct{}, len(prior))
	for _, snap := range history[1:] {
		for _, m := range snap.Maintainers {
			everSeen[strings.ToLower(strings.TrimSpace(m.Email))] = struct{}{}
		}
	}

	var newEmails []string
	for email := range current {
		if _, ok := prior[email]; ok {
			continue
		}
		newEmails = append(newEmails, email)
	}
	if len(newEmails) == 0 {
		return nil, nil
	}

	conf, ageHours := classifyConfidence(d.now(), newest.LatestVersionPublishedAt, newEmails, everSeen)
	expl := fmt.Sprintf(
		"new maintainer email(s) %v appeared on the latest snapshot of %s",
		newEmails, packageRef,
	)
	evidence := map[string]any{
		"new_emails":           newEmails,
		"prior_email_count":    len(prior),
		"current_email_count":  len(current),
		"latest_version":       newest.LatestVersion,
		"latest_publish_age_h": ageHours,
	}
	return []domain.Anomaly{{
		Kind:        d.Kind(),
		PackageRef:  packageRef,
		DetectedAt:  d.now(),
		Confidence:  conf,
		Explanation: expl,
		Evidence:    evidence,
	}}, nil
}

func emailSet(ms []domain.Maintainer) map[string]struct{} {
	out := make(map[string]struct{}, len(ms))
	for _, m := range ms {
		e := strings.ToLower(strings.TrimSpace(m.Email))
		if e == "" {
			continue
		}
		out[e] = struct{}{}
	}
	return out
}

// classifyConfidence picks a level by how recently the latest version
// was published and whether the new email is genuinely first-seen.
// Returns ageHours = -1 when LatestVersionPublishedAt is unknown.
func classifyConfidence(
	now time.Time,
	latestPublishedAt *time.Time,
	newEmails []string,
	everSeen map[string]struct{},
) (domain.ConfidenceLevel, float64) {
	ageHours := float64(-1)
	if latestPublishedAt != nil {
		ageHours = now.Sub(*latestPublishedAt).Hours()
	}

	allFirstSeen := true
	for _, e := range newEmails {
		if _, ok := everSeen[e]; ok {
			allFirstSeen = false
			break
		}
	}

	switch {
	case latestPublishedAt == nil:
		// No publish-time signal — we know about the drift but can't
		// say it was weaponised. Surface as low so a human triages.
		return domain.ConfidenceLow, ageHours
	case now.Sub(*latestPublishedAt) <= highWindow && allFirstSeen:
		return domain.ConfidenceHigh, ageHours
	case now.Sub(*latestPublishedAt) <= mediumWindow:
		return domain.ConfidenceMedium, ageHours
	default:
		return domain.ConfidenceLow, ageHours
	}
}
