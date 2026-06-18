SELECT
    n.id,
    n.type,
    COALESCE(n.dance_id, ''),
    COALESCE(n.reason, ''),
    n.is_read,
    n.created_at,
    COALESCE(n.from_user_id::text, ''),
    COALESCE(u.login, ''),
    COALESCE(n.ref_id, 0)
FROM notifications n
LEFT JOIN user_table u ON u.id = n.from_user_id
WHERE n.user_id = $1
ORDER BY n.created_at DESC
LIMIT 50;