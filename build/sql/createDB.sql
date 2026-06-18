-- Consolidated schema for the DDDance database.
-- This file folds in migrations 004–019 so a FRESH volume gets the complete
-- schema in one shot. The build/sql/migrations/*.sql files are still applied
-- afterwards by zzz_run_migrations.sh (all statements are idempotent), so an
-- already-initialised DB can be brought up to date incrementally — but a brand
-- new DB no longer depends on them to have e.g. the duels table.

CREATE TABLE IF NOT EXISTS user_table (
    id            uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY,
    version       integer DEFAULT 1 NOT NULL,
    login         text NOT NULL UNIQUE,
    vkid          text DEFAULT '',
    password_hash bytea NOT NULL,
    avatar        text DEFAULT 'avatars/default.png',
    has_2fa       boolean DEFAULT false,
    secret_code   text DEFAULT NULL,
    telegram_id   BIGINT DEFAULT NULL,
    telegram_link_code         text DEFAULT NULL,        -- migration 021
    telegram_link_code_expires timestamp with time zone, -- migration 021
    created_at    timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at    timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT user_table_login_check CHECK (length(login) >= 6 AND length(login) <= 20),
    CONSTRAINT user_table_password_hash_check CHECK (octet_length(password_hash) = 40)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_telegram_id ON user_table(telegram_id) WHERE telegram_id IS NOT NULL;
-- migration 010: unique vkid for non-empty values
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_vkid ON user_table(vkid) WHERE vkid != '' AND vkid IS NOT NULL;

CREATE TABLE IF NOT EXISTS dances (
    id                TEXT PRIMARY KEY,
    title             TEXT NOT NULL DEFAULT '',
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'processing', 'private', 'published', 'rejected')),
    difficulty        TEXT NOT NULL DEFAULT 'medium',
    difficulty_score  INTEGER NOT NULL DEFAULT 0,
    video_path        TEXT NOT NULL DEFAULT '',
    moderation_reason TEXT,
    duration_sec      INTEGER NOT NULL DEFAULT 0,          -- migration 013
    genre             TEXT NOT NULL DEFAULT 'other',       -- migration 019
    created_at        TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_dances_status ON dances(status);
-- migration 008: partial index for reels feed (published dances by recency)
CREATE INDEX IF NOT EXISTS idx_dances_published_created
    ON dances(created_at DESC)
    WHERE status = 'published';

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
    attempt_id      uuid NOT NULL PRIMARY KEY,
    user_id         UUID             NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    dance_id        TEXT             NOT NULL,
    score           DOUBLE PRECISION NOT NULL DEFAULT 0,
    has_video       BOOLEAN          NOT NULL DEFAULT FALSE,
    user_name       TEXT             NOT NULL DEFAULT '',
    is_private      BOOLEAN          NOT NULL DEFAULT FALSE,
    timing_score    FLOAT            NOT NULL DEFAULT 0,    -- migration 018
    amplitude_score FLOAT            NOT NULL DEFAULT 0,    -- migration 018
    pose_score      FLOAT            NOT NULL DEFAULT 0,    -- migration 018
    created_at      TIMESTAMPTZ      NOT NULL DEFAULT NOW()
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

-- migration 004 + 015: duels and duel invites.
-- Defined before notifications because notifications.duel_id references duels(id).
CREATE TABLE IF NOT EXISTS duels (
    id                        UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    mode                      TEXT        NOT NULL CHECK (mode IN ('single_dance', 'random_dance')),
    challenger_id             UUID        NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    opponent_id               UUID        NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    dance_id                  TEXT        REFERENCES dances(id) ON DELETE SET NULL,
    status                    TEXT        NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('pending', 'active', 'challenger_done', 'opponent_done', 'completed', 'expired', 'declined')),
    challenger_attempt_id     UUID        REFERENCES saved_attempts(attempt_id) ON DELETE SET NULL,
    opponent_attempt_id       UUID        REFERENCES saved_attempts(attempt_id) ON DELETE SET NULL,
    challenger_score          FLOAT,
    opponent_score            FLOAT,
    winner_id                 UUID        REFERENCES user_table(id) ON DELETE SET NULL,
    is_public                 BOOL        NOT NULL DEFAULT FALSE,   -- migration 015
    challenger_public_consent BOOL        NOT NULL DEFAULT FALSE,   -- migration 015
    opponent_public_consent   BOOL        NOT NULL DEFAULT FALSE,   -- migration 015
    expires_at                TIMESTAMPTZ NOT NULL,
    created_at                TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at              TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_duels_challenger_id ON duels(challenger_id);
CREATE INDEX IF NOT EXISTS idx_duels_opponent_id   ON duels(opponent_id);
CREATE INDEX IF NOT EXISTS idx_duels_status        ON duels(status);
CREATE INDEX IF NOT EXISTS idx_duels_expires_at    ON duels(expires_at);

CREATE TABLE IF NOT EXISTS duel_invites (
    id         UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    duel_id    UUID        NOT NULL REFERENCES duels(id) ON DELETE CASCADE,
    token      TEXT        NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- migrations 006/011/012: full notification type set + duel_id column.
CREATE TABLE IF NOT EXISTS notifications (
    id           BIGSERIAL   PRIMARY KEY,
    user_id      UUID        NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    type         TEXT        NOT NULL CHECK (type IN (
                     'dance_approved', 'dance_rejected',
                     'friend_request', 'friend_accepted', 'friend_declined',
                     'duel_challenge_received', 'duel_accepted', 'duel_declined',
                     'duel_challenger_done', 'duel_opponent_done', 'duel_completed', 'duel_expired')),
    dance_id     TEXT,
    duel_id      UUID        REFERENCES duels(id) ON DELETE SET NULL,   -- migration 012
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

-- migration 007: per-segment choreographer descriptions.
CREATE TABLE IF NOT EXISTS segment_descriptions (
    dance_id      TEXT        NOT NULL,
    segment_index INT         NOT NULL,
    description   TEXT        NOT NULL DEFAULT '',
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (dance_id, segment_index)
);
CREATE INDEX IF NOT EXISTS idx_segment_descriptions_dance
    ON segment_descriptions(dance_id);

-- migrations 005 + 017: achievements catalog and per-user unlocks.
CREATE TABLE IF NOT EXISTS achievements (
    id          SERIAL PRIMARY KEY,
    code        TEXT UNIQUE NOT NULL,
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    icon_key    TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL,
    threshold   INT  NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS user_achievements (
    user_id        uuid NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    achievement_id INT  NOT NULL REFERENCES achievements(id) ON DELETE CASCADE,
    unlocked_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, achievement_id)
);
CREATE INDEX IF NOT EXISTS user_achievements_user_id_idx ON user_achievements(user_id);

INSERT INTO achievements (code, title, description, icon_key, category, threshold) VALUES
('first_blood',      'Первая кровь',         'Получить 1 лайк на свой танец',                 'first_blood',      'likes',       1),
('crowd_pleaser',    'Нравится толпе',        'Получить 10 лайков на танцы',                   'crowd_pleaser',    'likes',       10),
('fan_favorite',     'Любимчик фанатов',      'Получить 50 лайков на танцы',                   'fan_favorite',     'likes',       50),
('half_score',       'Неплохо',               'Получить скор выше 50 в сравнении',             'half_score',       'score',       50),
('almost_perfect',   'Почти идеально',        'Получить скор выше 70 в сравнении',             'almost_perfect',   'score',       70),
('perfectionist',    'Перфекционист',         'Получить скор выше 90 в сравнении',             'perfectionist',    'score',       90),
('first_upload',     'Первый шаг',            'Загрузить первый танец',                        'first_upload',     'upload',      1),
('choreographer',    'Хореограф',             'Загрузить 5 танцев',                            'choreographer',    'upload',      5),
('dance_library',    'Библиотека движений',   'Загрузить 20 танцев',                           'dance_library',    'upload',      20),
('just_try',         'Просто попробуй',       'Сделать первую попытку',                        'just_try',         'attempt',     1),
('persistent',       'Настойчивый',           'Сделать 10 попыток',                            'persistent',       'attempt',     10),
('grinder',          'Шлифовщик',             'Сделать 50 попыток',                            'grinder',          'attempt',     50),
('first_duel',       'Первая дуэль',          'Принять участие в дуэли',                       'first_duel',       'duel',        1),
('duel_winner',      'Дуэлянт',               'Победить в 3 дуэлях',                           'duel_winner',      'duel_win',    3),
('duel_champion',    'Чемпион дуэлей',        'Победить в 10 дуэлях',                          'duel_champion',    'duel_win',    10),
('night_dancer',     'Ночной танцор',         'Загрузить попытку между 00:00 и 05:00',         'night_dancer',     'special',     1),
('speed_learner',    'Быстрый ученик',        'Пройти танец за один день после загрузки',      'speed_learner',    'special',     1),
('variety_dancer',   'Разносторонний',        'Попробовать 5 разных танцев',                   'variety_dancer',   'special',     5),
('social_butterfly', 'Социальная бабочка',    'Получить вызов на дуэль от 3 разных игроков',  'social_butterfly', 'special',     3),
('top_dancer',       'Топ танцор',            'Войти в топ-10 сайта',                          'top_dancer',       'special',     10),
('duel_streak_3',    'Серия побед',           'Одержать 3 победы в дуэлях подряд',             'duel_streak_3',    'duel_streak', 3),
('duel_streak_5',    'Непобедимый',           'Одержать 5 побед в дуэлях подряд',              'duel_streak_5',    'duel_streak', 5)
ON CONFLICT (code) DO NOTHING;

-- migration 016: friend activity feed.
CREATE TABLE IF NOT EXISTS activity_feed (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id        UUID        NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    action_type     TEXT        NOT NULL,
    metadata        JSONB       NOT NULL DEFAULT '{}',
    source_event_id TEXT,                                -- migration 020
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_activity_feed_actor_created ON activity_feed(actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_activity_feed_created ON activity_feed(created_at DESC);
-- migration 020: dedup Kafka redeliveries. UNIQUE allows multiple NULLs in
-- Postgres, so pre-existing rows (NULL source_event_id) are unaffected.
CREATE UNIQUE INDEX IF NOT EXISTS idx_activity_feed_source_event ON activity_feed(source_event_id);
