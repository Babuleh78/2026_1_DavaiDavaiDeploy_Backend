SELECT
    CASE WHEN f.sender_id = $1 THEN f.receiver_id ELSE f.sender_id END AS friend_id,
    u.login,
    COALESCE(u.avatar, '') AS avatar,
    f.updated_at AS friended_at,
    ad.id::text AS active_duel_id
FROM friendships f
JOIN user_table u ON u.id = CASE WHEN f.sender_id = $1 THEN f.receiver_id ELSE f.sender_id END
LEFT JOIN LATERAL (
    SELECT d.id FROM duels d
    WHERE (d.challenger_id = u.id OR d.opponent_id = u.id)
      AND d.status IN ('active', 'pending')
    LIMIT 1
) ad ON TRUE
WHERE (f.sender_id = $1 OR f.receiver_id = $1)
  AND f.status = 'accepted'
ORDER BY f.updated_at DESC
