UPDATE compare_tasks
SET finalized = TRUE
WHERE task_id = $1 AND finalized = FALSE;
