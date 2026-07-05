CREATE TABLE work_items (
    id text PRIMARY KEY,
    title text NOT NULL,
    description text NOT NULL DEFAULT '',
    source_type text NOT NULL DEFAULT '',
    source_id text NOT NULL DEFAULT '',
    actor text NOT NULL DEFAULT '',
    state text NOT NULL,
    priority integer NOT NULL DEFAULT 0,
    metadata jsonb,
    inserted_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL
);

CREATE INDEX work_items_state_idx ON work_items (state);
CREATE INDEX work_items_inserted_at_idx ON work_items (inserted_at DESC);
CREATE INDEX work_items_source_idx ON work_items (source_type, source_id);
