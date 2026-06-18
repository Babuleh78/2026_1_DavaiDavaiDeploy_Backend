SELECT
    u.login,
    COALESCE(u.avatar, '') AS avatar_url,
    MAX(da.score) AS best_score
FROM friendships f
JOIN user_table u ON u.id = CASE WHEN f.sender_id = $1 THEN f.receiver_id ELSE f.sender_id END
JOIN dance_attempts da ON da.user_id = u.id AND da.dance_id = $2
WHERE (f.sender_id = $1 OR f.receiver_id = $1)
  AND f.status = 'accepted'
GROUP BY u.id, u.login, u.avatar
ORDER BY best_score DESC
LIMIT 50;
