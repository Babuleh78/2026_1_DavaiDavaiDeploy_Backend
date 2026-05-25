SELECT dl.dance_id, COUNT(*) AS likes_count
FROM dance_likes dl
LEFT JOIN dances d ON d.id = dl.dance_id
WHERE d.id IS NULL OR d.status = 'published'
GROUP BY dl.dance_id
ORDER BY likes_count DESC
LIMIT $1;
