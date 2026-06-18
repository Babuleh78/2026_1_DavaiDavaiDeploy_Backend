SELECT created_at::date::text AS day, COUNT(*)::int AS count
FROM dance_attempts
WHERE user_id = $1
  AND created_at > now() - '1 year'::interval
GROUP BY day
ORDER BY day
LIMIT 365;
