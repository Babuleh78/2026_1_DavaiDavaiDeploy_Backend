-- DECISION: d.is_published and d.description do not exist in the schema.
-- Uses d.status='published' and derives description from segment_descriptions.
-- Uses dance_attempts for avg_score and dance_views for view_count (no comparison_results table).
WITH attempts_agg AS (
    SELECT dance_id,
           COALESCE(AVG(score), 0) AS avg_score
    FROM dance_attempts
    GROUP BY dance_id
),
views_agg AS (
    SELECT dance_id, COUNT(*) AS view_count
    FROM dance_views
    GROUP BY dance_id
),
segment_texts AS (
    SELECT dance_id,
           STRING_AGG(description, ' ' ORDER BY segment_index) AS description
    FROM segment_descriptions
    WHERE description != ''
    GROUP BY dance_id
)
SELECT
    d.id,
    d.title,
    COALESCE(st.description, '')       AS description,
    COALESCE(a.avg_score, 0)::float    AS avg_score,
    COALESCE(v.view_count, 0)::bigint  AS view_count,
    COALESCE(fu.user_id::text, '')     AS uploader_id,
    d.created_at
FROM dances d
LEFT JOIN attempts_agg a  ON a.dance_id = d.id
LEFT JOIN views_agg v     ON v.dance_id = d.id
LEFT JOIN segment_texts st ON st.dance_id = d.id
LEFT JOIN LATERAL (
    SELECT user_id FROM dance_uploads
    WHERE dance_id = d.id
    ORDER BY created_at ASC
    LIMIT 1
) fu ON TRUE
WHERE d.status = 'published'
ORDER BY d.created_at DESC
LIMIT 500
