SELECT
    COALESCE(COUNT(a.id), 0)::bigint         AS attempt_count,
    COALESCE(AVG(a.score), 0)::float         AS avg_score,
    COALESCE(MAX(a.score), 0)::float         AS top_score,
    COALESCE(
        (SELECT u.login
         FROM user_table u
         JOIN dance_attempts ta ON ta.user_id = u.id
         WHERE ta.dance_id = $1 AND ta.user_id IS NOT NULL
         ORDER BY ta.score DESC
         LIMIT 1
        ), ''
    ) AS top_user,
    COALESCE(
        (SELECT COUNT(*) FROM dance_views WHERE dance_id = $1),
        0
    )::bigint AS view_count
FROM dance_attempts a
WHERE a.dance_id = $1;
