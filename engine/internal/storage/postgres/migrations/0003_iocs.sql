-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS iocs (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL,
    severity     TEXT NOT NULL,
    ecosystem    TEXT NOT NULL DEFAULT '',
    source       TEXT NOT NULL DEFAULT '',
    published_at TIMESTAMPTZ NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    body         JSONB NOT NULL
);

CREATE INDEX IF NOT EXISTS iocs_published_idx ON iocs (published_at);
CREATE INDEX IF NOT EXISTS iocs_ecosystem_idx ON iocs (ecosystem);
CREATE INDEX IF NOT EXISTS iocs_kind_idx      ON iocs (kind);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS iocs;
-- +goose StatementEnd
