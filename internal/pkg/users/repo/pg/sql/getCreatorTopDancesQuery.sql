SELECT
    d.id  AS dance_id,
    d.title,
    COUNT(DISTINCT da.id)::int      AS attempts,
    COUNT(DISTINCT dl.user_id)::int AS likes
FROM dance_uploads du
JOIN dances d ON d.id = du.dance_id
LEFT JOIN dance_attempts da ON da.dance_id = d.id
LEFT JOIN dance_likes    dl ON dl.dance_id = d.id
WHERE du.user_id = $1
GROUP BY d.id, d.title
ORDER BY attempts DESC
LIMIT 3;
