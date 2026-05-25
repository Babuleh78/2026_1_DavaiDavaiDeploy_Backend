SELECT
    ids.id,
    COALESCE(d.title, '') AS title,
    COUNT(a.id)::bigint                AS attempt_count,
    COALESCE(AVG(a.score), 0)::float   AS avg_score,
    COALESCE(v.view_count, 0)::bigint  AS view_count,
    COALESCE(l.like_count, 0)::bigint  AS like_count
FROM unnest($1::text[]) AS ids(id)
LEFT JOIN dances d ON d.id = ids.id
LEFT JOIN dance_attempts a ON a.dance_id = ids.id
LEFT JOIN (
    SELECT dance_id, COUNT(*) AS view_count
    FROM dance_views
    GROUP BY dance_id
) v ON v.dance_id = ids.id
LEFT JOIN (
    SELECT dance_id, COUNT(DISTINCT user_id) AS like_count
    FROM dance_likes
    GROUP BY dance_id
) l ON l.dance_id = ids.id
GROUP BY ids.id, d.title, v.view_count, l.like_count;
