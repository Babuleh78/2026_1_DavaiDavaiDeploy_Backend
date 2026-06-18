INSERT INTO saved_attempts (attempt_id, user_id, dance_id, score, has_video, user_name, is_private, timing_score, amplitude_score, pose_score)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
ON CONFLICT (attempt_id) DO UPDATE SET
    score           = EXCLUDED.score,
    has_video       = EXCLUDED.has_video,
    user_name       = EXCLUDED.user_name,
    is_private      = EXCLUDED.is_private,
    timing_score    = EXCLUDED.timing_score,
    amplitude_score = EXCLUDED.amplitude_score,
    pose_score      = EXCLUDED.pose_score,
    created_at      = NOW();
