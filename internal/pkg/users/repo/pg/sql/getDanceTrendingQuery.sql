WITH attempts AS (
    SELECT dance_id, COUNT(*) AS attempt_count, AVG(score) AS avg_score
    FROM dance_attempts
    WHERE created_at >= NOW() - INTERVAL '7 days'
    GROUP BY dance_id
),
views AS (
    SELECT dance_id, COUNT(*) AS view_count
    FROM dance_views
    WHERE viewed_at >= NOW() - INTERVAL '7 days'
    GROUP BY dance_id
),
likes AS (
    SELECT dance_id, COUNT(DISTINCT user_id) AS like_count
    FROM dance_likes
    GROUP BY dance_id
)
SELECT
    d.id,
    d.title,
    a.attempt_count::bigint            AS attempt_count,
    COALESCE(a.avg_score, 0)::float    AS avg_score,
    COALESCE(v.view_count, 0)::bigint  AS view_count,
    COALESCE(l.like_count, 0)::bigint  AS like_count
FROM dances d
JOIN attempts a ON a.dance_id = d.id
LEFT JOIN views v ON v.dance_id = d.id
LEFT JOIN likes l ON l.dance_id = d.id
WHERE d.status = 'published'
ORDER BY a.attempt_count DESC
LIMIT 6;
