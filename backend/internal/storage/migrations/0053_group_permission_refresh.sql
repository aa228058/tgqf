-- Bounded, fenced refresh scheduling for shared group permission snapshots.
ALTER TABLE public_groups
    ADD COLUMN permissions_check_lease_until TIMESTAMPTZ,
    ADD COLUMN permissions_check_tenant_id UUID,
    ADD COLUMN permissions_check_account_id UUID,
    ADD COLUMN permissions_next_check_at TIMESTAMPTZ,
    ADD COLUMN permissions_check_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN permissions_check_error TEXT;

CREATE INDEX public_groups_permissions_check_due_idx
    ON public_groups(permissions_next_check_at NULLS FIRST, last_seen_at DESC, id)
    WHERE permissions_check_lease_until IS NULL OR permissions_check_lease_until <= NOW();
