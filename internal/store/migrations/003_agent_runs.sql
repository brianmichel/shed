CREATE TABLE agent_runs (
    id text PRIMARY KEY,
    work_item_id text NOT NULL REFERENCES work_items (id) ON DELETE CASCADE,
    sandbox_id text REFERENCES sandboxes (id) ON DELETE SET NULL,
    harness text NOT NULL DEFAULT '',
    model text NOT NULL DEFAULT '',
    state text NOT NULL,
    prompt text NOT NULL DEFAULT '',
    actor text NOT NULL DEFAULT '',
    metadata jsonb,
    started_at timestamptz,
    completed_at timestamptz,
    inserted_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX agent_runs_work_item_id_inserted_at_idx ON agent_runs (work_item_id, inserted_at DESC);
CREATE INDEX agent_runs_state_idx ON agent_runs (state);
CREATE INDEX agent_runs_sandbox_id_idx ON agent_runs (sandbox_id);
