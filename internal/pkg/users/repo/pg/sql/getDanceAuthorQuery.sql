SELECT u.id, u.login, u.avatar
FROM dance_uploads du
JOIN user_table u ON u.id = du.user_id
WHERE du.dance_id = $1
ORDER BY du.created_at ASC
LIMIT 1;
