INSERT INTO search_history (user_id, dance_id, source_url)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;