-- Migration 011: add duel_challenger_done to notifications.type CHECK constraint.
ALTER TABLE notifications
    DROP CONSTRAINT IF EXISTS notifications_type_check;

ALTER TABLE notifications
    ADD CONSTRAINT notifications_type_check CHECK (type IN (
        'dance_approved', 'dance_rejected',
        'friend_request', 'friend_accepted', 'friend_declined',
        'duel_challenge_received', 'duel_accepted', 'duel_declined',
        'duel_challenger_done', 'duel_opponent_done', 'duel_completed', 'duel_expired'
    ));
