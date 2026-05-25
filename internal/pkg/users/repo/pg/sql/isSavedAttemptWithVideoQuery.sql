SELECT EXISTS (
    SELECT 1 FROM saved_attempts
    WHERE attempt_id = $1
      AND user_id    = $2
      AND has_video  = TRUE
);
