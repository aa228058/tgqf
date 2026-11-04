CREATE TABLE IF NOT EXISTS telegram_incoming_update_dedup (
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    telegram_account_id UUID NOT NULL,
    chat_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    first_received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, telegram_account_id, chat_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_telegram_incoming_update_dedup_received
    ON telegram_incoming_update_dedup(first_received_at);
