WITH per_dance AS (
    SELECT
        dance_id,
        score,
        ROW_NUMBER() OVER (PARTITION BY dance_id ORDER BY created_at ASC)  AS rn,
        ROW_NUMBER() OVER (PARTITION BY dance_id ORDER BY created_at DESC) AS rn_desc
    FROM dance_attempts
    WHERE user_id = $1 AND attempt_id IS NOT NULL
),
scores AS (
    SELECT
        dance_id,
        MAX(score) FILTER (WHERE rn = 1)      AS first_score,
        MAX(score) FILTER (WHERE rn_desc = 1) AS last_score
    FROM per_dance
    GROUP BY dance_id
    HAVING COUNT(*) >= 2
)
SELECT s.dance_id, COALESCE(d.title, '') AS title, s.first_score, s.last_score,
       (s.last_score - s.first_score) AS delta
FROM scores s
JOIN dances d ON d.id = s.dance_id
WHERE s.last_score > s.first_score
ORDER BY delta DESC
LIMIT 1;
