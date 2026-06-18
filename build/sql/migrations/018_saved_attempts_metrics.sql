-- Add per-metric score columns so GET /api/users/me/weak-spots can compute
-- per-dimension averages without reading S3 result files.
ALTER TABLE saved_attempts
    ADD COLUMN IF NOT EXISTS timing_score    FLOAT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS amplitude_score FLOAT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS pose_score      FLOAT NOT NULL DEFAULT 0;
