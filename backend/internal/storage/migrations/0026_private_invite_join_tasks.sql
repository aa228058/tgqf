ALTER TABLE tenant_join_tasks
    ADD COLUMN private_invite_hash_ciphertext BYTEA;

ALTER TABLE tenant_join_tasks
    DROP CONSTRAINT IF EXISTS tenant_join_tasks_check;

ALTER TABLE tenant_join_tasks
    ADD CONSTRAINT tenant_join_tasks_target_check
    CHECK (
        (target_group_id IS NOT NULL OR target_link_id IS NOT NULL OR private_invite_hash_ciphertext IS NOT NULL)
        AND NOT (
            private_invite_hash_ciphertext IS NOT NULL
            AND (target_group_id IS NOT NULL OR target_link_id IS NOT NULL)
        )
    );
