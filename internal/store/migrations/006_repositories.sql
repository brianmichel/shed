CREATE TABLE repositories (
    id text PRIMARY KEY,
    name text NOT NULL,
    provider text NOT NULL DEFAULT 'git',
    clone_url text NOT NULL,
    default_branch text NOT NULL DEFAULT 'main',
    credential_ref text NOT NULL DEFAULT '',
    metadata jsonb,
    inserted_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX repositories_provider_idx ON repositories (provider);
CREATE INDEX repositories_inserted_at_idx ON repositories (inserted_at DESC);
CREATE UNIQUE INDEX repositories_provider_clone_url_idx ON repositories (provider, clone_url);
