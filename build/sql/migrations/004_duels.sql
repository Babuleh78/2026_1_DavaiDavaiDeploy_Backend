CREATE TABLE IF NOT EXISTS duels (
    id                     UUID        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    mode                   TEXT        NOT NULL CHECK (mode IN ('single_dance', 'random_dance')),
    challenger_id          UUID        NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    opponent_id            UUID        NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    dance_id               TEXT        REFERENCES dances(id) ON DELETE SET NULL,
    status                 TEXT        NOT NULL DEFAULT 'pending'
                           CHECK (status IN ('pending', 'active', 'challenger_done', 'opponent_done', 'completed', 'expired', 'declined')),
    challenger_attempt_id  UUID        REFERENCES saved_attempts(attempt_id) ON DELETE SET NULL,
    opponent_attempt_id    UUID        REFERENCES saved_attempts(attempt_id) ON DELETE SET NULL,
    challenger_score       FLOAT,
    opponent_score         FLOAT,
    winner_id              UUID        REFERENCES user_table(id) ON DELETE SET NULL,
    expires_at             TIMESTAMPTZ NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at           TIMESTAMPTZ
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
