SELECT user_id
FROM dance_attempts
WHERE attempt_id = $1
LIMIT 1;
