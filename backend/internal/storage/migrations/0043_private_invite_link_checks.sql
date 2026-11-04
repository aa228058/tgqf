ALTER TABLE public_group_links
 ADD COLUMN last_checked_at TIMESTAMPTZ,
 ADD COLUMN last_check_error TEXT;

CREATE INDEX public_group_links_check_due_idx
 ON public_group_links(last_checked_at NULLS FIRST, id)
 WHERE link_type='invite';
