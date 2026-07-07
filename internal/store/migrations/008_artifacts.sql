CREATE TABLE artifacts (
    id text PRIMARY KEY,
    work_item_id text NOT NULL REFERENCES work_items (id) ON DELETE CASCADE,
    agent_run_id text REFERENCES agent_runs (id) ON DELETE SET NULL,
    sandbox_id text REFERENCES sandboxes (id) ON DELETE SET NULL,
    type text NOT NULL,
    uri text NOT NULL,
    content_hash text NOT NULL DEFAULT '',
    metadata jsonb,
    inserted_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX artifacts_work_item_id_inserted_at_idx ON artifacts (work_item_id, inserted_at DESC);
CREATE INDEX artifacts_agent_run_id_inserted_at_idx ON artifacts (agent_run_id, inserted_at DESC);
CREATE INDEX artifacts_type_idx ON artifacts (type);
