-- Ordered, tenant-owned keyword action sequences.
ALTER TABLE tenant_keyword_rules
    ADD COLUMN IF NOT EXISTS action_steps JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(action_steps)='array');

ALTER TABLE keyword_action_jobs
    ADD COLUMN IF NOT EXISTS action_steps JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(action_steps)='array'),
    ADD COLUMN IF NOT EXISTS current_step_index INTEGER NOT NULL DEFAULT 0 CHECK (current_step_index >= 0),
    ADD COLUMN IF NOT EXISTS step_results JSONB NOT NULL DEFAULT '[]'::jsonb
        CHECK (jsonb_typeof(step_results)='array');

CREATE INDEX IF NOT EXISTS keyword_action_jobs_sequence_progress_idx
    ON keyword_action_jobs(tenant_id,status,available_at)
    WHERE status IN ('pending','processing');
