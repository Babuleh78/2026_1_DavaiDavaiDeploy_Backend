SELECT
    u.id::text       AS user_id,
    u.login          AS username,
    u.avatar         AS avatar,
    AVG(sa.score)    AS avg_score,
    COUNT(*)         AS attempt_count,
    MAX(sa.score)    AS best_score
FROM saved_attempts sa
JOIN user_table u ON u.id = sa.user_id
WHERE sa.is_private = FALSE
  AND u.id::text = ANY($1)
GROUP BY u.id, u.login, u.avatar
HAVING COUNT(*) >= 3
ORDER BY best_score DESC;
