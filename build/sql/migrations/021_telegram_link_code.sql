-- migration 021: secure Telegram account linking via one-time code.
-- The web app (authenticated) issues a short-lived code; the bot redeems it to
-- bind telegram_id to the real account. Replaces the insecure "type a nickname"
-- login (/bot/auth), which let anyone impersonate any user.
ALTER TABLE user_table ADD COLUMN IF NOT EXISTS telegram_link_code TEXT;
ALTER TABLE user_table ADD COLUMN IF NOT EXISTS telegram_link_code_expires TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_users_tg_link_code
    ON user_table(telegram_link_code) WHERE telegram_link_code IS NOT NULL;
