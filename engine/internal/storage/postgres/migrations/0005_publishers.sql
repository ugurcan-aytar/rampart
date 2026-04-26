-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS publishers (
    ecosystem  TEXT NOT NULL,
    name       TEXT NOT NULL,
    first_seen TIMESTAMPTZ NOT NULL,
    last_seen  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (ecosystem, name)
);

CREATE TABLE IF NOT EXISTS publisher_profiles (
    ecosystem           TEXT NOT NULL,
    name                TEXT NOT NULL,
    package_count       INT  NOT NULL DEFAULT 0,
    publish_count       INT  NOT NULL DEFAULT 0,
    last_30day_publishes INT NOT NULL DEFAULT 0,
    uses_oidc           BOOLEAN NOT NULL DEFAULT FALSE,
    has_git_tags        BOOLEAN NOT NULL DEFAULT FALSE,
    maintainer_emails   TEXT[] NOT NULL DEFAULT '{}',
    last_email_change   TIMESTAMPTZ,
    PRIMARY KEY (ecosystem, name),
    FOREIGN KEY (ecosystem, name) REFERENCES publishers(ecosystem, name) ON DELETE CASCADE
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS publisher_profiles;
DROP TABLE IF EXISTS publishers;
-- +goose StatementEnd
