SELECT 
    sh.id as history_id,
    dl.dance_id,
    sh.name,
    dl.created_at
FROM dance_likes dl
LEFT JOIN search_history sh 
    ON sh.dance_id = dl.dance_id AND sh.user_id = dl.user_id
WHERE dl.user_id = $1
ORDER BY dl.created_at DESC;