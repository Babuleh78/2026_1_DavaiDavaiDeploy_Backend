SELECT COALESCE(moderation_reason, '') FROM dances WHERE id = $1;
