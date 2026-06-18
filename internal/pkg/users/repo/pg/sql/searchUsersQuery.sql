SELECT id, login, avatar
FROM user_table
WHERE login ILIKE '%' || $1 || '%'
ORDER BY
    CASE
        WHEN LOWER(login) = LOWER($1) THEN 0
        WHEN login ILIKE $1 || '%' THEN 1
        ELSE 2
    END,
    login
LIMIT $2;
