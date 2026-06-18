-- Partial index for reels feed: published dances ordered by recency
CREATE INDEX IF NOT EXISTS idx_dances_published_created
    ON dances(created_at DESC)
    WHERE status = 'published';
