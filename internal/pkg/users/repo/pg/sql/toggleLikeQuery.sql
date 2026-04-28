INSERT INTO dance_likes (user_id, dance_id)
VALUES ($1, $2)
ON CONFLICT (user_id, dance_id) DO NOTHING
RETURNING true AS inserted; 