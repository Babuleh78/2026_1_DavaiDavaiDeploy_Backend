WITH views_agg AS (
    SELECT dance_id, COUNT(*) AS view_count
    FROM dance_views
    GROUP BY dance_id
),
likes_agg AS (
    SELECT dance_id, COUNT(DISTINCT user_id) AS like_count
    FROM dance_likes
    GROUP BY dance_id
),
attempts_agg AS (
    SELECT dance_id,
           COUNT(*) AS attempt_count,
           AVG(score) AS avg_score
    FROM dance_attempts
    GROUP BY dance_id
),
first_uploader AS (
    SELECT DISTINCT ON (dance_id) dance_id, user_id
    FROM dance_uploads
    ORDER BY dance_id, created_at ASC
)
SELECT
    d.id,
    d.title,
    fu.user_id::text                         AS uploader_id,
    u.login                                  AS username,
    u.avatar                                 AS avatar_url,
    d.video_path                             AS video_url,
    ''::text                                 AS preview_url,
    COALESCE(v.view_count, 0)::bigint        AS view_count,
    COALESCE(l.like_count, 0)::bigint        AS like_count,
    COALESCE(a.avg_score, 0)::float          AS avg_score,
    COALESCE(a.attempt_count, 0)::bigint     AS attempt_count,
    CASE
        WHEN $4::uuid IS NULL THEN false
        ELSE EXISTS (
            SELECT 1 FROM dance_likes dl
            WHERE dl.dance_id = d.id AND dl.user_id = $4::uuid
        )
    END AS user_liked
FROM dances d
JOIN first_uploader fu ON fu.dance_id = d.id
JOIN user_table u ON u.id = fu.user_id
LEFT JOIN views_agg v ON v.dance_id = d.id
LEFT JOIN likes_agg l ON l.dance_id = d.id
LEFT JOIN attempts_agg a ON a.dance_id = d.id
WHERE d.status = 'published'
  AND ($3::text[] IS NULL OR d.id != ALL($3::text[]))
ORDER BY d.created_at DESC
LIMIT $1 OFFSET $2;
