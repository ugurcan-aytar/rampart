package domain

import (
	"time"
)

// AnomalyKind names a class of publisher anomaly that Theme F2's
// detectors can raise. The string values intentionally mirror the
// matching `SignalType` constants from publisher.go so a single
// vocabulary spans the Theme F surface — `SignalType` is the broader
// catalog (7 values, some still detector-less); `AnomalyKind` is the
// F2 subset that ships with concrete detectors.
//
// String values are persisted in the `anomalies.kind` column and
// surfaced over the API; changing one is a breaking contract change.
type AnomalyKind string

const (
	AnomalyKindMaintainerEmailDrift     AnomalyKind = "new_maintainer_email"
	AnomalyKindOIDCPublishingRegression AnomalyKind = "oidc_regression"
	AnomalyKindVersionJump              AnomalyKind = "version_jump"
)

// ConfidenceLevel grades a detector's certainty. The detector picks the
// level based on how many corroborating signals fired — see each
// detector's package doc for the exact thresholds.
type ConfidenceLevel string

const (
	ConfidenceLow    ConfidenceLevel = "low"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceHigh   ConfidenceLevel = "high"
)

// Anomaly is a single detector hit. PackageRef carries the same
// `<ecosystem>:<name>` shape as PublisherSnapshot.PackageRef so the
// two layers join naturally on (PackageRef, time-window).
//
// Evidence is detector-specific structured data — JSONB on the wire,
// `map[string]any` in Go. Detectors document their evidence shape in
// their package doc.
type Anomaly struct {
	ID          int64
	Kind        AnomalyKind
	PackageRef  string
	DetectedAt  time.Time
	Confidence  ConfidenceLevel
	Explanation string
	Evidence    map[string]any
}

// AnomalyFilter scopes a ListAnomalies query. Empty fields are
// "no filter on this dimension". From / To are inclusive on both sides.
type AnomalyFilter struct {
	PackageRef string
	Kind       AnomalyKind
	From       *time.Time
	To         *time.Time
	Limit      int // 0 = no cap (storage applies a sentinel max)
}
