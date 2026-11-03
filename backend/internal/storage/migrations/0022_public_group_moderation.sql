CREATE TABLE public_group_corrections (
    id BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES public_groups(id) ON DELETE CASCADE,
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    submitted_by_user_id UUID NOT NULL REFERENCES users(id),
    proposed_fields JSONB NOT NULL CHECK (jsonb_typeof(proposed_fields)='object'),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
    review_note TEXT NOT NULL DEFAULT '',
    reviewed_by_user_id UUID REFERENCES users(id),
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((status='pending' AND reviewed_by_user_id IS NULL AND reviewed_at IS NULL)
        OR (status IN ('approved','rejected') AND reviewed_by_user_id IS NOT NULL AND reviewed_at IS NOT NULL))
);
CREATE UNIQUE INDEX public_group_corrections_one_pending_idx
    ON public_group_corrections(group_id,tenant_id) WHERE status='pending';
CREATE INDEX public_group_corrections_review_idx
    ON public_group_corrections(status,created_at,id);

CREATE TABLE public_group_merge_audits (
    id BIGSERIAL PRIMARY KEY,
    source_group_id BIGINT NOT NULL,
    target_group_id BIGINT NOT NULL,
    actor_user_id UUID NOT NULL REFERENCES users(id),
    reason TEXT NOT NULL DEFAULT '',
    source_snapshot JSONB NOT NULL,
    merged_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (source_group_id<>target_group_id)
);
CREATE INDEX public_group_merge_audits_recent_idx ON public_group_merge_audits(merged_at DESC);
