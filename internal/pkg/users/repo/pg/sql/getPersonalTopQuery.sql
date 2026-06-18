WITH best_per_dance AS (
    SELECT DISTINCT ON (dance_id)
        dance_id,
        attempt_id,
        score,
        created_at,
        user_name
    FROM saved_attempts
    WHERE user_id = $1
    ORDER BY dance_id, score DESC, created_at DESC
)
SELECT
    b.dance_id,
    b.attempt_id,
    COALESCE(d.title, '') AS title,
    COALESCE(b.user_name, '') AS user_name,
    b.score      AS best_score,
    b.created_at AS last_attempt_at
FROM best_per_dance b
LEFT JOIN dances d ON d.id = b.dance_id
ORDER BY b.score DESC
LIMIT 3;
