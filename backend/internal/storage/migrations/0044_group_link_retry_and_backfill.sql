ALTER TABLE public_group_links
    ADD COLUMN check_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN next_check_at TIMESTAMPTZ;

-- Preserve the existing six-hour cooldown for links that were already checked.
UPDATE public_group_links
SET next_check_at = last_checked_at + INTERVAL '6 hours'
WHERE last_checked_at IS NOT NULL;

CREATE INDEX public_group_links_next_check_idx
    ON public_group_links(next_check_at NULLS FIRST, id);

-- Backfill links discovered before their public group entered the catalogue.
UPDATE public_group_links l
SET group_id = (
    SELECT g.id
    FROM public_groups g
    WHERE NULLIF(g.username, '') IS NOT NULL
      AND lower(l.public_url) IN (
          lower('https://t.me/' || g.username),
          lower('http://t.me/' || g.username),
          lower('https://telegram.me/' || g.username)
      )
    ORDER BY g.id
    LIMIT 1
)
WHERE l.link_type = 'public'
  AND l.group_id IS NULL
  AND EXISTS (
      SELECT 1 FROM public_groups g
      WHERE NULLIF(g.username, '') IS NOT NULL
        AND lower(l.public_url) IN (
            lower('https://t.me/' || g.username),
            lower('http://t.me/' || g.username),
            lower('https://telegram.me/' || g.username)
        )
  );
