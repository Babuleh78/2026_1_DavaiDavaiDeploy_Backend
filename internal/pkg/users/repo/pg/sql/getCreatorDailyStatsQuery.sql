WITH creator_dances AS (
    SELECT du.dance_id
    FROM dance_uploads du
    WHERE du.user_id = $1
),
days AS (
    SELECT generate_series(
        (current_date - interval '6 days')::date,
        current_date,
        interval '1 day'
    )::date AS day
),
daily_views AS (
    SELECT viewed_at::date AS day, COUNT(*)::int AS cnt
    FROM dance_views
    WHERE dance_id IN (SELECT dance_id FROM creator_dances)
      AND viewed_at >= current_date - interval '6 days'
    GROUP BY 1
),
daily_likes AS (
    SELECT created_at::date AS day, COUNT(*)::int AS cnt
    FROM dance_likes
    WHERE dance_id IN (SELECT dance_id FROM creator_dances)
      AND created_at >= current_date - interval '6 days'
    GROUP BY 1
),
daily_attempts AS (
    SELECT created_at::date AS day, COUNT(*)::int AS cnt
    FROM dance_attempts
    WHERE dance_id IN (SELECT dance_id FROM creator_dances)
      AND created_at >= current_date - interval '6 days'
    GROUP BY 1
)
SELECT
    d.day::text,
    COALESCE(dv.cnt, 0) AS views,
    COALESCE(dl.cnt, 0) AS likes,
    COALESCE(da.cnt, 0) AS attempts
FROM days d
LEFT JOIN daily_views    dv ON dv.day = d.day
LEFT JOIN daily_likes    dl ON dl.day = d.day
LEFT JOIN daily_attempts da ON da.day = d.day
ORDER BY d.day;
