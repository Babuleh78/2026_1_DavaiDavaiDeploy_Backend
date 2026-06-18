-- Migration 006: extend notifications.type CHECK to include duel notification types.
-- The auto-generated constraint name in PostgreSQL for an inline CHECK is
-- <table>_<column>_check, i.e. notifications_type_check.
ALTER TABLE notifications
    DROP CONSTRAINT IF EXISTS notifications_type_check;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_type_check CHECK (type IN (
        'dance_approved', 'dance_rejected',
        'friend_request', 'friend_accepted', 'friend_declined',
        'duel_challenge_received', 'duel_accepted', 'duel_declined',
        'duel_opponent_done', 'duel_completed', 'duel_expired'
    ));
