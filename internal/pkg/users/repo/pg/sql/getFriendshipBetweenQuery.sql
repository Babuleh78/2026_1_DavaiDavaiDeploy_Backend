SELECT id, status, sender_id
FROM friendships
WHERE (sender_id = $1 AND receiver_id = $2)
   OR (sender_id = $2 AND receiver_id = $1)
LIMIT 1
