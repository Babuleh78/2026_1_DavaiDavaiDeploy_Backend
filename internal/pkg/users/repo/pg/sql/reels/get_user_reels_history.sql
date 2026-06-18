-- DECISION: comparison_results table does not exist in the schema.
-- sa.score is used directly from saved_attempts instead of a join.
-- dance_views UNIONed in to broaden personalization signal (score=0, liked=false).
(
    SELECT
        sa.dance_id,
        sa.score,
        sa.created_at AS viewed_at,
        EXISTS(
            SELECT 1
            FROM dance_likes dl
            WHERE dl.dance_id = sa.dance_id
              AND dl.user_id = $1
        ) AS liked
    FROM saved_attempts sa
    WHERE sa.user_id = $1
    ORDER BY sa.created_at DESC
    LIMIT 50
)
UNION ALL
(
    SELECT
        dv.dance_id,
        0.0 AS score,
        dv.viewed_at,
        FALSE AS liked
    FROM dance_views dv
    WHERE dv.viewer_id = $1::text
    ORDER BY dv.viewed_at DESC
    LIMIT 100
)
ORDER BY viewed_at DESC
LIMIT 100
