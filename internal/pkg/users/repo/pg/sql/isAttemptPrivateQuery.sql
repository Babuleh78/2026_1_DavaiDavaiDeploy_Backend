-- Returns TRUE only when the attempt is a saved attempt marked private.
-- An attempt that is not saved (no row) is treated as not-private here: privacy
-- is a property the owner sets on saved attempts shown in their profile.
SELECT COALESCE((SELECT is_private FROM saved_attempts WHERE attempt_id = $1), FALSE);
