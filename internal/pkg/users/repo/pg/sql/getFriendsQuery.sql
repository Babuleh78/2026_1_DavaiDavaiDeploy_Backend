SELECT
    CASE WHEN f.sender_id = $1 THEN f.receiver_id ELSE f.sender_id END AS friend_id,
    u.login,
    COALESCE(u.avatar, '') AS avatar,
    f.updated_at AS friended_at
FROM friendships f
JOIN user_table u ON u.id = CASE WHEN f.sender_id = $1 THEN f.receiver_id ELSE f.sender_id END
WHERE (f.sender_id = $1 OR f.receiver_id = $1)
  AND f.status = 'accepted'
ORDER BY f.updated_at DESC
