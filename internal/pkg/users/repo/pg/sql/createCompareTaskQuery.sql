INSERT INTO compare_tasks (task_id, dance_id, user_dance_id, video_key)
VALUES ($1, $2, $3, $4)
ON CONFLICT (task_id) DO NOTHING;
