SELECT COUNT(*)
FROM friendships
WHERE (sender_id = $1 OR receiver_id = $1)
  AND status = 'accepted'
