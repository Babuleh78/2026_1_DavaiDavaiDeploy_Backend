INSERT INTO dances (id, title, status, difficulty, video_path)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (id) DO NOTHING;
