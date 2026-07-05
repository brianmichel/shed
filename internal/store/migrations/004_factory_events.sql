CREATE TABLE factory_event_sequences (
    work_item_id text PRIMARY KEY REFERENCES work_items (id) ON DELETE CASCADE,
    next_seq bigint NOT NULL
);

CREATE TABLE factory_events (
    id text PRIMARY KEY,
    work_item_id text NOT NULL REFERENCES work_items (id) ON DELETE CASCADE,
    agent_run_id text REFERENCES agent_runs (id) ON DELETE SET NULL,
    seq bigint NOT NULL,
    type text NOT NULL,
    source text NOT NULL DEFAULT '',
    timestamp timestamptz NOT NULL,
    data jsonb,
    UNIQUE (work_item_id, seq)
);

CREATE INDEX factory_events_work_item_id_seq_idx ON factory_events (work_item_id, seq ASC);
CREATE INDEX factory_events_work_item_id_agent_run_id_seq_idx ON factory_events (work_item_id, agent_run_id, seq ASC);
CREATE INDEX factory_events_type_idx ON factory_events (type);
