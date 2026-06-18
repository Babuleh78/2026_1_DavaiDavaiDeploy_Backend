-- Add a dedicated duel_id column to notifications so duel notifications
-- do not misuse the dance_id column.
ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS duel_id UUID REFERENCES duels(id) ON DELETE SET NULL;
