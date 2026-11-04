-- Each tenant/account refreshes its own rights snapshot independently.
-- Shared default permissions remain on public_groups; leases live on the
-- tenant-scoped source row so one tenant cannot starve another source account.
ALTER TABLE public_group_sources
    ADD COLUMN rights_check_lease_until TIMESTAMPTZ,
    ADD COLUMN rights_next_check_at TIMESTAMPTZ,
    ADD COLUMN rights_check_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN rights_check_error TEXT;

CREATE INDEX public_group_sources_rights_due_idx
    ON public_group_sources(rights_next_check_at NULLS FIRST, rights_check_lease_until, tenant_id, telegram_account_id, group_id);
