DELETE FROM search_history
WHERE user_id = $1
AND (
    created_at < NOW() - INTERVAL '14 days'
    OR id IN (
        SELECT id FROM search_history
        WHERE user_id = $1
        ORDER BY created_at DESC
        OFFSET 25
    )
);