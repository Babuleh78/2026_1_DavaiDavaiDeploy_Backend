DELETE FROM search_history
WHERE id = $1 AND user_id = $2;