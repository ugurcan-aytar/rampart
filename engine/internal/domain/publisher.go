package domain

import "time"

// Publisher is a maintainer identity on a package registry. The tuple
// (Ecosystem, Name) is the primary key.
type Publisher struct {
	Ecosystem string
	Name      string
	FirstSeen time.Time
	LastSeen  time.Time
}

// PublisherProfile aggregates baseline data the trust engine uses to detect
// anomalies. Phase 3 detectors fill this in from historical publish metadata.
type PublisherProfile struct {
	Publisher          Publisher
	PackageCount       int
	PublishCount       int
	Last30DayPublishes int
	UsesOIDC           bool
	HasGitTags         bool
	MaintainerEmails   []string
	LastEmailChange    *time.Time
}

// SignalType names a class of anomaly a trust Detector can raise.
// The string values are persisted — changing them is a breaking change.
type SignalType string

const (
	SignalNewMaintainerEmail   SignalType = "new_maintainer_email"
	SignalDormantAccountActive SignalType = "dormant_account_active"
	SignalMissingGitTag        SignalType = "missing_git_tag"
	SignalOffHoursPublish      SignalType = "off_hours_publish"
	SignalOIDCRegression       SignalType = "oidc_regression"
	SignalVersionJump          SignalType = "version_jump"
	SignalLowDownloadDayAttack SignalType = "low_download_day_attack"
)

// PublisherSignal is a single raised anomaly, tied to a publisher and
// optionally to the package that prompted it.
type PublisherSignal struct {
	Type        SignalType
	Publisher   Publisher
	Package     *PackageVersion
	Severity    Severity
	Description string
	Evidence    map[string]any
	DetectedAt  time.Time
}
