ALTER TABLE user_table ADD COLUMN IF NOT EXISTS telegram_id BIGINT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_telegram_id ON user_table(telegram_id) WHERE telegram_id IS NOT NULL;
