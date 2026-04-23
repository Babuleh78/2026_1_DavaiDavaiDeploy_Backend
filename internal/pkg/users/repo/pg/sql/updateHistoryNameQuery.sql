UPDATE search_history
SET name = $1
WHERE id = $2 AND user_id = $3;