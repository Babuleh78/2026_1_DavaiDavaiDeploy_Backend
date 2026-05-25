UPDATE notifications
SET is_read = TRUE
WHERE user_id = $1 AND is_read = FALSE;
