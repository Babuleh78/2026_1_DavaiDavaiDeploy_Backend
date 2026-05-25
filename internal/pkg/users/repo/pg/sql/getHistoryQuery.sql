SELECT
    sh.id, sh.user_id, sh.dance_id, sh.name, sh.source_url, sh.created_at,
    COALESCE(d.title, '') AS dance_title,
    (SELECT score FROM dance_attempts WHERE dance_id = sh.dance_id AND user_id = sh.user_id ORDER BY created_at DESC LIMIT 1) AS score
FROM search_history sh
LEFT JOIN dances d ON d.id = sh.dance_id
WHERE sh.user_id = $1
ORDER BY sh.created_at DESC
LIMIT 50;
