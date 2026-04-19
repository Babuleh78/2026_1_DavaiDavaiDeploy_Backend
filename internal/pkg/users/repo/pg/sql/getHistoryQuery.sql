SELECT id, user_id, dance_id, name, source_url, created_at
FROM search_history
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT 50;