-- migration 020: deduplicate activity_feed entries on Kafka redelivery.
-- The activity-feed-worker now writes a stable source_event_id per logical event
-- and inserts with ON CONFLICT (source_event_id) DO NOTHING. A UNIQUE index is
-- enough: Postgres treats NULLs as distinct, so rows written before this
-- migration (NULL source_event_id) are left untouched.
ALTER TABLE activity_feed ADD COLUMN IF NOT EXISTS source_event_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_activity_feed_source_event ON activity_feed(source_event_id);
