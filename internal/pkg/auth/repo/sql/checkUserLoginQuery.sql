SELECT
    id, version, login, password_hash, avatar, created_at, updated_at,
    CASE
        WHEN vkid IS NOT NULL AND vkid != '' THEN true
        ELSE false
    END AS is_foreign
FROM user_table
WHERE login = $1