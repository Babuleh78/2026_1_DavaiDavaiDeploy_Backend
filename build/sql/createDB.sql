CREATE TABLE IF NOT EXISTS user_table (
    id uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY, 
    version integer DEFAULT 1 NOT NULL,
    login text NOT NULL,
    vkid text DEFAULT '',
    password_hash bytea NOT NULL,
    avatar text DEFAULT 'avatars/default.png',
    has_2fa boolean DEFAULT false, 
    secret_code text DEFAULT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT user_table_login_check CHECK (((length(login) >= 6) AND (length(login) <= 20))),
    CONSTRAINT user_table_password_hash_check CHECK ((octet_length(password_hash) = 40))
);

CREATE TABLE IF NOT EXISTS search_history (
    id          uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    dance_id    text NOT NULL,
    name        text DEFAULT 'Без названия',
    source_url  text DEFAULT '',
    created_at  timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


CREATE TABLE IF NOT EXISTS dance_likes (
    user_id  uuid NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    dance_id text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, dance_id)
);

CREATE INDEX IF NOT EXISTS dance_likes_dance_id_idx ON dance_likes(dance_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_history_user_dance 
ON search_history (user_id, dance_id);