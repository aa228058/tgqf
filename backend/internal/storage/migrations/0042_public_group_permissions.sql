-- Distinguish shared group defaults from each tenant account's own rights.
ALTER TABLE public_groups
    ADD COLUMN default_permissions JSONB,
    ADD COLUMN permissions_observed_at TIMESTAMPTZ;

ALTER TABLE public_group_sources
    ADD COLUMN account_rights_snapshot JSONB,
    ADD COLUMN account_rights_observed_at TIMESTAMPTZ;

CREATE INDEX public_groups_permissions_observed_idx
    ON public_groups(permissions_observed_at DESC) WHERE permissions_observed_at IS NOT NULL;
CREATE INDEX public_group_sources_rights_observed_idx
    ON public_group_sources(tenant_id, account_rights_observed_at DESC) WHERE account_rights_observed_at IS NOT NULL;
