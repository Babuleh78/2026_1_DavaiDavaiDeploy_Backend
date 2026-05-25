INSERT INTO friendships (sender_id, receiver_id)
VALUES ($1, $2)
RETURNING id
