DELETE FROM saved_attempts
WHERE attempt_id = $1 AND user_id = $2;
