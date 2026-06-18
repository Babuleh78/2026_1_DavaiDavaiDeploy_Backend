UPDATE notifications
SET is_read = TRUE
WHERE id = $1 AND user_id = $2;