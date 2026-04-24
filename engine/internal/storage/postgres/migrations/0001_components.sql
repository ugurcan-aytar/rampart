-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS components (
    ref         TEXT PRIMARY KEY,
    kind        TEXT NOT NULL,
    namespace   TEXT NOT NULL,
    name        TEXT NOT NULL,
    owner       TEXT NOT NULL DEFAULT '',
    system      TEXT NOT NULL DEFAULT '',
    lifecycle   TEXT NOT NULL DEFAULT '',
    tags        TEXT[] NOT NULL DEFAULT '{}',
    annotations JSONB  NOT NULL DEFAULT '{}'::jsonb
);

CREATE INDEX IF NOT EXISTS components_owner_idx     ON components (owner);
CREATE INDEX IF NOT EXISTS components_namespace_idx ON components (namespace);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS components;
-- +goose StatementEnd
