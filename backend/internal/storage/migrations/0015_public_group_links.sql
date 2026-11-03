CREATE TABLE public_group_links (
 id BIGSERIAL PRIMARY KEY,
 group_id BIGINT REFERENCES public_groups(id) ON DELETE SET NULL,
 link_type TEXT NOT NULL CHECK (link_type IN ('public','invite')),
 public_url TEXT,
 invite_hash BYTEA,
 invite_ciphertext BYTEA,
 status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','valid','invalid','approval_required','unknown')),
 first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE(link_type,public_url),
 UNIQUE(invite_hash),
 CHECK ((link_type='public' AND public_url IS NOT NULL AND invite_hash IS NULL AND invite_ciphertext IS NULL) OR (link_type='invite' AND public_url IS NULL AND invite_hash IS NOT NULL AND invite_ciphertext IS NOT NULL))
);
CREATE TABLE public_group_link_sources (
 link_id BIGINT NOT NULL REFERENCES public_group_links(id) ON DELETE CASCADE,
 tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
 telegram_account_id UUID NOT NULL,
 source_chat_id BIGINT NOT NULL,
 source_message_id BIGINT NOT NULL,
 first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 discovery_count BIGINT NOT NULL DEFAULT 1 CHECK(discovery_count>0),
 PRIMARY KEY(link_id,tenant_id,telegram_account_id,source_chat_id,source_message_id),
 FOREIGN KEY(tenant_id,telegram_account_id) REFERENCES telegram_accounts(tenant_id,id) ON DELETE CASCADE
);
CREATE INDEX public_group_links_status_idx ON public_group_links(status,last_seen_at DESC);
CREATE INDEX public_group_link_sources_tenant_idx ON public_group_link_sources(tenant_id,last_seen_at DESC);
