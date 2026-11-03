ALTER TABLE telegram_accounts ALTER COLUMN phone DROP NOT NULL;
ALTER TABLE telegram_accounts DROP CONSTRAINT IF EXISTS telegram_accounts_tenant_id_phone_key;
CREATE UNIQUE INDEX telegram_accounts_tenant_phone_unique ON telegram_accounts(tenant_id, phone) WHERE phone IS NOT NULL AND phone <> '';
