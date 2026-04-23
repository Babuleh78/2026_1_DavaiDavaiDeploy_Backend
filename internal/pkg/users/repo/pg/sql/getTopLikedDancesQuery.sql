SELECT dance_id, COUNT(*) as likes_count
FROM dance_likes
GROUP BY dance_id
ORDER BY likes_count DESC
LIMIT $1;