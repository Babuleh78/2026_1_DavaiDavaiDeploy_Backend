SELECT af.id::text, af.action_type, af.metadata, af.created_at,
       u.login AS actor_login, COALESCE(u.avatar, '') AS actor_avatar,
       COALESCE(d.title, '') AS dance_title
FROM activity_feed af
JOIN user_table u ON u.id = af.actor_id
LEFT JOIN dances d ON d.id = af.metadata->>'dance_id'
WHERE af.actor_id IN (
    SELECT CASE WHEN sender_id = $1 THEN receiver_id ELSE sender_id END
    FROM friendships
    WHERE (sender_id = $1 OR receiver_id = $1) AND status = 'accepted'
)
AND af.created_at < $3
ORDER BY af.created_at DESC
LIMIT $2
