UPDATE friendships
SET status = $3, updated_at = NOW()
WHERE id = $1 AND receiver_id = $2
RETURNING sender_id
