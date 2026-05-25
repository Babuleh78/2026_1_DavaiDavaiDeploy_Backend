CREATE TABLE IF NOT EXISTS user_table (
    id            uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY,
    version       integer DEFAULT 1 NOT NULL,
    login         text NOT NULL,
    vkid          text DEFAULT '',
    password_hash bytea NOT NULL,
    avatar        text DEFAULT 'avatars/default.png',
    has_2fa       boolean DEFAULT false,
    secret_code   text DEFAULT NULL,
    created_at    timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at    timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT user_table_login_check CHECK (length(login) >= 6 AND length(login) <= 20),
    CONSTRAINT user_table_password_hash_check CHECK (octet_length(password_hash) = 40)
);

CREATE TABLE IF NOT EXISTS dances (
    id                TEXT PRIMARY KEY,
    title             TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'processing', 'private', 'published', 'rejected')),
    difficulty        TEXT NOT NULL DEFAULT 'medium',
    difficulty_score  INTEGER NOT NULL DEFAULT 0,
    video_path        TEXT NOT NULL DEFAULT '',
    moderation_reason TEXT,
    created_at        TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dances_status ON dances(status);

CREATE TABLE IF NOT EXISTS dance_uploads (
    user_id    UUID        NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    dance_id   TEXT        NOT NULL REFERENCES dances(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, dance_id)
);
CREATE INDEX IF NOT EXISTS idx_dance_uploads_user ON dance_uploads(user_id);
CREATE INDEX IF NOT EXISTS idx_dance_uploads_dance ON dance_uploads(dance_id);

CREATE TABLE IF NOT EXISTS dance_attempts (
    id         BIGSERIAL PRIMARY KEY,
    attempt_id uuid UNIQUE,
    dance_id   TEXT  NOT NULL,
    user_id    UUID,
    score      FLOAT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dance_attempts_dance_id ON dance_attempts(dance_id);
CREATE INDEX IF NOT EXISTS idx_dance_attempts_created_at ON dance_attempts(created_at);
CREATE INDEX IF NOT EXISTS idx_dance_attempts_user_dance_score
    ON dance_attempts(user_id, dance_id, score DESC);

CREATE TABLE IF NOT EXISTS saved_attempts (
    attempt_id uuid NOT NULL PRIMARY KEY,
    user_id    UUID             NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    dance_id   TEXT             NOT NULL,
    score      DOUBLE PRECISION NOT NULL DEFAULT 0,
    has_video  BOOLEAN          NOT NULL DEFAULT FALSE,
    user_name  TEXT             NOT NULL DEFAULT '',
    is_private BOOLEAN          NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_saved_attempts_user_created
    ON saved_attempts(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_saved_attempts_user_dance
    ON saved_attempts(user_id, dance_id);

CREATE TABLE IF NOT EXISTS dance_likes (
    user_id    UUID NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    dance_id   TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, dance_id)
);
CREATE INDEX IF NOT EXISTS dance_likes_dance_id_idx ON dance_likes(dance_id);

CREATE TABLE IF NOT EXISTS dance_ratings (
    id            uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY,
    video_id      text NOT NULL,
    user_id       uuid NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    physical      integer NOT NULL CHECK (physical BETWEEN 1 AND 10),
    speed         integer NOT NULL CHECK (speed BETWEEN 1 AND 10),
    coordination  integer NOT NULL CHECK (coordination BETWEEN 1 AND 10),
    repeatability integer NOT NULL CHECK (repeatability BETWEEN 1 AND 10),
    created_at    TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (video_id, user_id)
);
CREATE INDEX IF NOT EXISTS dance_ratings_video_id_idx ON dance_ratings(video_id);
CREATE INDEX IF NOT EXISTS dance_ratings_user_id_idx ON dance_ratings(user_id);

CREATE TABLE IF NOT EXISTS search_history (
    id         uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    dance_id   text NOT NULL,
    name       text DEFAULT 'Без названия',
    source_url text DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_history_user_dance
    ON search_history(user_id, dance_id);

CREATE TABLE IF NOT EXISTS dance_features (
    id                      uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY,
    video_id                text NOT NULL UNIQUE,
    angular_velocity_avg    float,
    angular_velocity_max    float,
    center_of_mass_variance float,
    simultaneous_limbs_avg  float,
    pose_entropy            float,
    jump_count              integer,
    rotation_count          integer,
    created_at              TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS dance_views (
    dance_id   TEXT        NOT NULL,
    viewer_id  TEXT        NOT NULL,
    viewed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (dance_id, viewer_id)
);
CREATE INDEX IF NOT EXISTS idx_dance_views_dance_created
    ON dance_views(dance_id, viewed_at DESC);

CREATE TABLE IF NOT EXISTS friendships (
    id          BIGSERIAL   PRIMARY KEY,
    sender_id   UUID        NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    receiver_id UUID        NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    status      TEXT        NOT NULL DEFAULT 'pending'
                CHECK (status IN ('pending', 'accepted', 'declined')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (sender_id, receiver_id)
);
CREATE INDEX IF NOT EXISTS idx_friendships_receiver ON friendships(receiver_id, status);
CREATE INDEX IF NOT EXISTS idx_friendships_sender   ON friendships(sender_id, status);

CREATE TABLE IF NOT EXISTS notifications (
    id           BIGSERIAL   PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    type         TEXT        NOT NULL CHECK (type IN (
                     'dance_approved', 'dance_rejected',
                     'friend_request', 'friend_accepted', 'friend_declined')),
    dance_id     TEXT,
    reason       TEXT,
    from_user_id UUID        REFERENCES user_table(id) ON DELETE SET NULL,
    ref_id       BIGINT,
    is_read      BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notifications_user_created
    ON notifications(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
    ON notifications(user_id) WHERE is_read = FALSE;

CREATE TABLE IF NOT EXISTS compare_tasks (
    task_id       TEXT PRIMARY KEY,
    dance_id      TEXT NOT NULL,
    user_dance_id TEXT NOT NULL,
    video_key     TEXT NOT NULL,
    finalized     BOOLEAN NOT NULL DEFAULT FALSE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);