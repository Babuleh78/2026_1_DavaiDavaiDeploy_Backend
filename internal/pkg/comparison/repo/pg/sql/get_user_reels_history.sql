-- DECISION: comparison_results table does not exist in the schema.
-- sa.score is used directly from saved_attempts instead of a join.
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
