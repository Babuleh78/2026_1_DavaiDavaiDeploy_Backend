DELETE FROM dance_likes
WHERE user_id = $1 AND dance_id = $2;