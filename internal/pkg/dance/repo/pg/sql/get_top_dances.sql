WITH attempts_agg AS (
    SELECT dance_id, COUNT(*) AS attempt_count, AVG(score) AS avg_score
    FROM dance_attempts
    GROUP BY dance_id
),
views_agg AS (
    SELECT dance_id, COUNT(*) AS view_count
    FROM dance_views
    GROUP BY dance_id
),
likes_agg AS (
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
JOIN attempts_agg a ON a.dance_id = d.id
LEFT JOIN views_agg v ON v.dance_id = d.id
LEFT JOIN likes_agg l ON l.dance_id = d.id
WHERE d.status = 'published'
ORDER BY avg_score DESC, view_count DESC, like_count DESC
LIMIT 20;
