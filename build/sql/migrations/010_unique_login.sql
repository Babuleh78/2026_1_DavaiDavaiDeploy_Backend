ALTER TABLE user_table ADD CONSTRAINT user_table_login_unique UNIQUE (login);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_vkid ON user_table(vkid) WHERE vkid != '' AND vkid IS NOT NULL;
