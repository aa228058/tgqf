ALTER TABLE public_group_links
    ADD COLUMN check_lease_until TIMESTAMPTZ,
    ADD COLUMN check_lease_tenant_id UUID,
    ADD COLUMN check_lease_account_id UUID;

CREATE INDEX public_group_links_check_lease_idx
    ON public_group_links(check_lease_until)
    WHERE check_lease_until IS NOT NULL;
