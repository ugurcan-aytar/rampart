-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS anomalies (
    id           BIGSERIAL PRIMARY KEY,
    kind         TEXT NOT NULL,
    package_ref  TEXT NOT NULL,
    detected_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    confidence   TEXT NOT NULL,
    explanation  TEXT NOT NULL,
    evidence     JSONB NOT NULL DEFAULT '{}'::jsonb,

    -- Idempotency guard: a detector that re-runs over the same
    -- snapshot history must not produce duplicate rows. The triple
    -- (kind, package_ref, detected_at) is the natural deduplication
    -- key — same anomaly raised at the same instant for the same
    -- package is, by construction, the same hit.
    UNIQUE (kind, package_ref, detected_at)
);

-- (package_ref, detected_at DESC) supports `GET /v1/anomalies?package_ref=…`
-- and the F3 IncidentDashboard surface.
CREATE INDEX IF NOT EXISTS idx_anomalies_package_ref_detected_at
    ON anomalies (package_ref, detected_at DESC);

-- (kind, detected_at DESC) supports `GET /v1/anomalies?kind=…` and
-- the future "global feed of recent anomalies of type X" view.
CREATE INDEX IF NOT EXISTS idx_anomalies_kind_detected_at
    ON anomalies (kind, detected_at DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS anomalies;
-- +goose StatementEnd
