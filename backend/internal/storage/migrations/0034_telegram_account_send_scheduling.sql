ALTER TABLE telegram_accounts
    ADD COLUMN IF NOT EXISTS telegram_send_next_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS telegram_send_cooldown_until TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS telegram_accounts_send_schedule_idx
    ON telegram_accounts(tenant_id, telegram_send_next_at, telegram_send_cooldown_until);
