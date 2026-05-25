SELECT dance_id, user_dance_id, video_key
FROM compare_tasks
WHERE task_id = $1;
