SELECT
    d.id,
    d.title,
    d.status,
    d.video_path,
    du.created_at,
    COALESCE(att.attempt_count, 0)::bigint AS attempt_count,
    COALESCE(att.avg_score, 0)::float      AS avg_score,
    COALESCE(lk.like_count, 0)::bigint     AS like_count,
    COALESCE(vc.view_count, 0)::bigint     AS view_count,
    (COALESCE(dr.rating_count, 0) >= %d)   AS difficulty_by_users,
    CASE WHEN COALESCE(dr.rating_count, 0) >= %d
         THEN ROUND((dr.avg_diff - 2) / 8.0 * 100)::int
         ELSE d.difficulty_score END       AS effective_difficulty_score
FROM dance_uploads du
JOIN dances d ON d.id = du.dance_id
LEFT JOIN (
    SELECT dance_id,
           COUNT(*)::bigint AS attempt_count,
           AVG(score)::float AS avg_score
    FROM dance_attempts GROUP BY dance_id
) att ON att.dance_id = d.id
LEFT JOIN (
    SELECT dance_id, COUNT(*)::bigint AS like_count
    FROM dance_likes GROUP BY dance_id
) lk ON lk.dance_id = d.id
LEFT JOIN (
    SELECT dance_id, COUNT(*)::bigint AS view_count
    FROM dance_views GROUP BY dance_id
) vc ON vc.dance_id = d.id
LEFT JOIN (
    SELECT video_id,
           COUNT(*)::int AS rating_count,
           AVG((physical + speed + coordination + repeatability) / 4.0) AS avg_diff
    FROM dance_ratings GROUP BY video_id
) dr ON dr.video_id = d.id
WHERE du.user_id = $1
ORDER BY du.created_at DESC;
