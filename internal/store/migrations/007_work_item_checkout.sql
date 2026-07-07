ALTER TABLE work_items ADD COLUMN repository_id text REFERENCES repositories (id) ON DELETE SET NULL;
ALTER TABLE work_items ADD COLUMN repository_ref text NOT NULL DEFAULT '';
ALTER TABLE work_items ADD COLUMN repository_base_branch text NOT NULL DEFAULT '';

CREATE INDEX work_items_repository_id_idx ON work_items (repository_id);
