SELECT
    sa.attempt_id,
    sa.dance_id,
    COALESCE(d.title, '') AS title,
    sa.score,
    sa.created_at,
    sa.has_video,
    sa.user_name,
    sa.is_private
FROM saved_attempts sa
LEFT JOIN dances d ON d.id = sa.dance_id
WHERE sa.user_id = $1
ORDER BY sa.created_at DESC
LIMIT $2 OFFSET $3;
