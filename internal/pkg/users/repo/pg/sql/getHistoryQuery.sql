-- History only surfaces dances that are actually watchable. The INNER JOIN with
-- status = 'published' drops dances that are still processing, awaiting/failed
-- moderation, private, or deleted (missing from dances) — those have no playable
-- results/{id}/video.mp4 and otherwise 502 on the client.
-- DECISION: gate on 'published' to match reels/top/catalog; non-published dances
-- are never publicly viewable, so they have no place in watch history.
-- One row per (user_id, dance_id) is guaranteed by idx_history_user_dance.
SELECT
    sh.id, sh.user_id, sh.dance_id, sh.name, sh.source_url, sh.created_at,
    COALESCE(d.title, '') AS dance_title,
    last_attempt.score
FROM search_history sh
JOIN dances d ON d.id = sh.dance_id AND d.status = 'published'
LEFT JOIN LATERAL (
    SELECT score FROM dance_attempts da
    WHERE da.dance_id = sh.dance_id AND da.user_id = sh.user_id
    ORDER BY da.created_at DESC LIMIT 1
) last_attempt ON true
WHERE sh.user_id = $1
ORDER BY sh.created_at DESC
LIMIT 50;
