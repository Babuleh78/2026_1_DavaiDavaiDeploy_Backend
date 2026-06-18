SELECT
    COALESCE(AVG(timing_score),    0)::float AS timing,
    COALESCE(AVG(amplitude_score), 0)::float AS amplitude,
    COALESCE(AVG(pose_score),      0)::float AS pose
FROM saved_attempts
WHERE user_id = $1
  AND (timing_score + amplitude_score + pose_score) > 0
  AND created_at > NOW() - '30 days'::interval;
