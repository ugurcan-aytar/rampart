-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS publisher_history (
    id                            BIGSERIAL PRIMARY KEY,
    package_ref                   TEXT NOT NULL,
    snapshot_at                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    maintainers                   JSONB NOT NULL DEFAULT '[]'::jsonb,
    latest_version                TEXT,
    latest_version_published_at   TIMESTAMPTZ,
    publish_method                TEXT,
    source_repo_url               TEXT,
    raw_data                      JSONB
);

-- (package_ref, snapshot_at DESC) is the access pattern used by both
-- the GET /v1/publisher/{ref}/history endpoint and the F2 anomaly
-- detector — fetching the latest N snapshots for a package.
CREATE INDEX IF NOT EXISTS idx_publisher_history_package_ref_snapshot_at
    ON publisher_history (package_ref, snapshot_at DESC);

-- (snapshot_at) supports ListPackagesNeedingRefresh — "give me packages
-- whose most-recent snapshot is older than X". The cron tick runs this
-- query every interval to pick which packages to re-ingest next.
CREATE INDEX IF NOT EXISTS idx_publisher_history_snapshot_at
    ON publisher_history (snapshot_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS publisher_history;
-- +goose StatementEnd
