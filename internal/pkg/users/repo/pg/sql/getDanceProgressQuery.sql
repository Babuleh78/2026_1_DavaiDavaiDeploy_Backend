SELECT attempt_id, score, created_at
FROM dance_attempts
WHERE user_id  = $1
  AND dance_id = $2
  AND attempt_id IS NOT NULL
ORDER BY created_at ASC
LIMIT 100;
