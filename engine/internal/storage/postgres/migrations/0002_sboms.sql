-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS sboms (
    id            TEXT PRIMARY KEY,
    component_ref TEXT NOT NULL,
    commit_sha    TEXT NOT NULL DEFAULT '',
    ecosystem     TEXT NOT NULL,
    generated_at  TIMESTAMPTZ NOT NULL,
    source_format TEXT NOT NULL DEFAULT '',
    source_bytes  BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS sboms_component_generated_idx
    ON sboms (component_ref, generated_at);

CREATE TABLE IF NOT EXISTS sbom_packages (
    sbom_id   TEXT NOT NULL REFERENCES sboms(id) ON DELETE CASCADE,
    position  INT  NOT NULL,
    ecosystem TEXT NOT NULL,
    name      TEXT NOT NULL,
    version   TEXT NOT NULL,
    purl      TEXT NOT NULL DEFAULT '',
    scope     TEXT[] NOT NULL DEFAULT '{}',
    integrity TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (sbom_id, position)
);

CREATE INDEX IF NOT EXISTS sbom_packages_name_version_idx
    ON sbom_packages (ecosystem, name, version);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS sbom_packages;
DROP TABLE IF EXISTS sboms;
-- +goose StatementEnd
