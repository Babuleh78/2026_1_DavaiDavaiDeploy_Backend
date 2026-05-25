INSERT INTO dance_uploads (user_id, dance_id)
VALUES ($1, $2)
ON CONFLICT (user_id, dance_id) DO NOTHING;
