UPDATE user_table
SET login = COALESCE($1, login),
    avatar = COALESCE($2, avatar),
    version = CASE WHEN $3::bool THEN version + 1 ELSE version END,
    updated_at = CURRENT_TIMESTAMP
WHERE id = $4
