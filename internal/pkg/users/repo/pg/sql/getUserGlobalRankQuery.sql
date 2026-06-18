SELECT COUNT(*) + 1
FROM (
    SELECT DISTINCT user_id
    FROM dance_attempts
    WHERE score > (
        SELECT COALESCE(MAX(score), -1)
        FROM dance_attempts
        WHERE user_id = $1
    )
) t;
