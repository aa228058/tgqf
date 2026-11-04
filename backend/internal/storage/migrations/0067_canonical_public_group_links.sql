-- Telegram public usernames are case-insensitive. Consolidate historical
-- case-variant URLs before enforcing one canonical shared link per username.
WITH ranked AS (
    SELECT id,
           FIRST_VALUE(id) OVER (PARTITION BY lower(public_url) ORDER BY id) AS keeper_id
    FROM public_group_links
    WHERE link_type='public'
), duplicate_sources AS (
    SELECT r.keeper_id,
           s.tenant_id,
           s.telegram_account_id,
           s.source_chat_id,
           s.source_message_id,
           MIN(s.first_seen_at) AS first_seen_at,
           MAX(s.last_seen_at) AS last_seen_at,
           SUM(s.discovery_count) AS discovery_count
    FROM ranked r
    JOIN public_group_link_sources s ON s.link_id=r.id
    WHERE r.id<>r.keeper_id
    GROUP BY r.keeper_id,s.tenant_id,s.telegram_account_id,s.source_chat_id,s.source_message_id
)
INSERT INTO public_group_link_sources(
    link_id,tenant_id,telegram_account_id,source_chat_id,source_message_id,
    first_seen_at,last_seen_at,discovery_count
)
SELECT keeper_id,tenant_id,telegram_account_id,source_chat_id,source_message_id,
       first_seen_at,last_seen_at,discovery_count
FROM duplicate_sources
ON CONFLICT(link_id,tenant_id,telegram_account_id,source_chat_id,source_message_id)
DO UPDATE SET
    first_seen_at=LEAST(public_group_link_sources.first_seen_at,EXCLUDED.first_seen_at),
    last_seen_at=GREATEST(public_group_link_sources.last_seen_at,EXCLUDED.last_seen_at),
    discovery_count=public_group_link_sources.discovery_count+EXCLUDED.discovery_count;

WITH ranked AS (
    SELECT id,
           FIRST_VALUE(id) OVER (PARTITION BY lower(public_url) ORDER BY id) AS keeper_id
    FROM public_group_links
    WHERE link_type='public'
)
UPDATE public_group_links keeper
SET group_id=COALESCE(keeper.group_id,duplicate.group_id),
    first_seen_at=LEAST(keeper.first_seen_at,duplicate.first_seen_at),
    last_seen_at=GREATEST(keeper.last_seen_at,duplicate.last_seen_at),
    status=CASE
        WHEN keeper.status IN ('valid','approval_required') THEN keeper.status
        WHEN duplicate.status IN ('valid','approval_required') THEN duplicate.status
        WHEN keeper.status='unknown' OR duplicate.status='unknown' THEN 'unknown'
        WHEN keeper.status='invalid' AND duplicate.status='pending' THEN 'pending'
        ELSE keeper.status
    END,
    last_checked_at=GREATEST(keeper.last_checked_at,duplicate.last_checked_at)
FROM ranked r
JOIN public_group_links duplicate ON duplicate.id=r.id
WHERE keeper.id=r.keeper_id AND duplicate.id<>r.keeper_id;

WITH ranked AS (
    SELECT id,
           FIRST_VALUE(id) OVER (PARTITION BY lower(public_url) ORDER BY id) AS keeper_id
    FROM public_group_links
    WHERE link_type='public'
)
DELETE FROM public_group_links duplicate
USING ranked r
WHERE duplicate.id=r.id AND r.id<>r.keeper_id;

UPDATE public_group_links
SET public_url='https://t.me/' || lower(regexp_replace(public_url,'^https?://(www\.)?(t\.me|telegram\.me)/','', 'i'))
WHERE link_type='public';

CREATE UNIQUE INDEX public_group_links_public_username_unique
    ON public_group_links(lower(public_url))
    WHERE link_type='public';
