SELECT
    da.attempt_id,
    da.dance_id,
    COALESCE(d.title, '')                                       AS title,
    da.score,
    da.created_at,
    EXISTS (
        SELECT 1 FROM saved_attempts sa
        WHERE sa.attempt_id = da.attempt_id
    )                                                            AS is_saved
FROM dance_attempts da
LEFT JOIN dances d ON d.id = da.dance_id
WHERE da.user_id    = $1
  AND da.attempt_id IS NOT NULL
ORDER BY da.created_at DESC;
