SELECT dance_id, created_at
FROM dance_likes
WHERE user_id = $1
ORDER BY created_at DESC;