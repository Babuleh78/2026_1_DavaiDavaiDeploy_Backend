WITH attempts_agg AS (
    SELECT
        COALESCE(COUNT(a.id), 0)::bigint AS attempt_count,
        COALESCE(AVG(a.score), 0)::float AS avg_score,
        COALESCE(MAX(a.score), 0)::float AS top_score
    FROM dance_attempts a
    WHERE a.dance_id = $1
),
top_user AS (
    SELECT u.login AS top_user
    FROM user_table u
    JOIN dance_attempts ta ON ta.user_id = u.id
    WHERE ta.dance_id = $1 AND ta.user_id IS NOT NULL
    ORDER BY ta.score DESC
    LIMIT 1
),
views AS (
    SELECT COUNT(*)::bigint AS view_count
    FROM dance_views
    WHERE dance_id = $1
)
SELECT
    aa.attempt_count,
    aa.avg_score,
    aa.top_score,
    COALESCE(tu.top_user, '') AS top_user,
    COALESCE(v.view_count, 0) AS view_count
FROM attempts_agg aa
LEFT JOIN top_user tu ON true
CROSS JOIN views v;
