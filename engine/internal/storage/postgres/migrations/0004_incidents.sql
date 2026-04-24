-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS incidents (
    id                            TEXT PRIMARY KEY,
    ioc_id                        TEXT NOT NULL,
    state                         TEXT NOT NULL,
    opened_at                     TIMESTAMPTZ NOT NULL,
    last_transitioned_at          TIMESTAMPTZ NOT NULL,
    affected_components_snapshot  TEXT[] NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS incidents_state_idx    ON incidents (state);
CREATE INDEX IF NOT EXISTS incidents_opened_idx   ON incidents (opened_at);
CREATE INDEX IF NOT EXISTS incidents_ioc_idx      ON incidents (ioc_id);

CREATE TABLE IF NOT EXISTS remediations (
    id          TEXT PRIMARY KEY,
    incident_id TEXT NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    executed_at TIMESTAMPTZ NOT NULL,
    actor_ref   TEXT NOT NULL DEFAULT '',
    details     JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- `seq` preserves insertion order independently of clock skew so
    -- ListRemediations(incidentID) can return rows exactly as appended.
    seq         BIGSERIAL NOT NULL
);

CREATE INDEX IF NOT EXISTS remediations_incident_seq_idx
    ON remediations (incident_id, seq);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS remediations;
DROP TABLE IF EXISTS incidents;
-- +goose StatementEnd
