-- A duel can only reference an attempt that already lives in saved_attempts
-- (duels.challenger_attempt_id / opponent_attempt_id FK). When a user submits an
-- attempt to a duel without having explicitly saved it to their profile, the row
-- is missing and the FK update fails. This persists it idempotently.
-- DO NOTHING (not DO UPDATE) so an already-saved attempt keeps its real
-- sub-scores / is_private / user_name untouched.
INSERT INTO saved_attempts (attempt_id, user_id, dance_id, score, has_video, is_private)
VALUES ($1, $2, $3, $4, TRUE, $5)
ON CONFLICT (attempt_id) DO NOTHING;
