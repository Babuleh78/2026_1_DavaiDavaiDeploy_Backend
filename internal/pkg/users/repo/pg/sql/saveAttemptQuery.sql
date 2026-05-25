INSERT INTO saved_attempts (attempt_id, user_id, dance_id, score, has_video, user_name, is_private)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (attempt_id) DO UPDATE SET
    score      = EXCLUDED.score,
    has_video  = EXCLUDED.has_video,
    user_name  = EXCLUDED.user_name,
    is_private = EXCLUDED.is_private,
    created_at = NOW();
