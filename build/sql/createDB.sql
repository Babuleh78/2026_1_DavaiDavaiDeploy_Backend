CREATE TABLE IF NOT EXISTS user_table (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
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