SELECT
    sa.attempt_id,
    sa.dance_id,
    COALESCE(d.title, '')   AS dance_title,
    sa.user_id::text        AS user_id,
    COALESCE(u.login, '')   AS user_login,
    COALESCE(u.avatar, '')  AS user_avatar,
    sa.score
FROM saved_attempts sa
JOIN user_table u ON u.id = sa.user_id
LEFT JOIN dances d ON d.id = sa.dance_id
WHERE sa.is_private = false
  AND sa.has_video  = true
ORDER BY sa.score DESC
LIMIT $1 OFFSET $2;
