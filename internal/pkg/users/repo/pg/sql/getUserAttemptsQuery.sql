WITH user_dance_ids AS (
    SELECT DISTINCT dance_id
    FROM dance_attempts
    WHERE user_id = $1 AND attempt_id IS NOT NULL
),
ranked AS (
    SELECT
        da.dance_id,
        da.user_id,
        DENSE_RANK() OVER (PARTITION BY da.dance_id ORDER BY MAX(da.score) DESC)::int AS rank,
        COUNT(*) OVER (PARTITION BY da.dance_id)::int                                  AS total_dancers
    FROM dance_attempts da
    WHERE da.user_id IS NOT NULL
      AND da.dance_id IN (SELECT dance_id FROM user_dance_ids)
    GROUP BY da.dance_id, da.user_id
)
SELECT
    da.attempt_id,
    da.dance_id,
    COALESCE(d.title, '')                                       AS title,
    da.score,
    da.created_at,
    EXISTS (
        SELECT 1 FROM saved_attempts sa
        WHERE sa.attempt_id = da.attempt_id
    )                                                           AS is_saved,
    EXISTS (
        SELECT 1 FROM saved_attempts sa
        WHERE sa.attempt_id = da.attempt_id AND sa.is_private = FALSE
    )                                                           AS is_open,
    COALESCE(
        (SELECT sa.user_name FROM saved_attempts sa WHERE sa.attempt_id = da.attempt_id),
        ''
    )                                                           AS user_name,
    r.rank,
    r.total_dancers
FROM dance_attempts da
LEFT JOIN dances d ON d.id = da.dance_id
LEFT JOIN ranked r ON r.dance_id = da.dance_id AND r.user_id = $1
WHERE da.user_id    = $1
  AND da.attempt_id IS NOT NULL
ORDER BY da.created_at DESC
LIMIT $2 OFFSET $3;
