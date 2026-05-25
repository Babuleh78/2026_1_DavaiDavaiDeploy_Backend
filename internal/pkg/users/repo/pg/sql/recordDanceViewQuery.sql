INSERT INTO dance_views (dance_id, viewer_id)
VALUES ($1, $2)
ON CONFLICT (dance_id, viewer_id) DO NOTHING
