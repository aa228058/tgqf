-- System-wide public Telegram group/channel catalogue.
CREATE TABLE public_groups (
    id BIGSERIAL PRIMARY KEY,
    telegram_chat_id BIGINT NOT NULL,
    chat_type TEXT NOT NULL CHECK (chat_type IN ('group','channel')),
    title TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    member_count INTEGER NOT NULL DEFAULT 0 CHECK (member_count >= 0),
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(chat_type, telegram_chat_id)
);

CREATE TABLE public_group_sources (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES public_groups(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    telegram_account_id UUID NOT NULL,
    source_chat_id BIGINT NOT NULL,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    discovery_count BIGINT NOT NULL DEFAULT 1 CHECK (discovery_count > 0),
    UNIQUE(group_id, tenant_id, telegram_account_id),
    FOREIGN KEY (tenant_id, telegram_account_id) REFERENCES telegram_accounts(tenant_id, id) ON DELETE CASCADE
);

CREATE TABLE tenant_group_contributions (
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES public_groups(id) ON DELETE CASCADE,
    first_contributed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_contributed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    contribution_count BIGINT NOT NULL DEFAULT 1 CHECK (contribution_count > 0),
    PRIMARY KEY (tenant_id, group_id)
);

CREATE INDEX public_groups_recent_idx ON public_groups(last_seen_at DESC, title ASC);
CREATE INDEX public_groups_search_idx ON public_groups(chat_type, lower(title), lower(username));
CREATE INDEX public_group_sources_tenant_idx ON public_group_sources(tenant_id, last_seen_at DESC);
CREATE INDEX tenant_group_contributions_group_idx ON tenant_group_contributions(group_id, last_contributed_at DESC);
