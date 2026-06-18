WITH attempts_agg AS (
    SELECT dance_id, AVG(score) AS avg_score
    FROM dance_attempts
    GROUP BY dance_id
),
views_agg AS (
    SELECT dance_id, COUNT(*) AS view_count
    FROM dance_views
    GROUP BY dance_id
),
segment_texts AS (
    SELECT dance_id, STRING_AGG(description, ' ' ORDER BY segment_index) AS description
    FROM segment_descriptions
    WHERE description != ''
    GROUP BY dance_id
)
SELECT
    d.id,
    d.title,
    COALESCE(st.description, '')    AS description,
    COALESCE(a.avg_score, 0)::float AS avg_score,
    COALESCE(v.view_count, 0)::bigint AS view_count
FROM dances d
LEFT JOIN attempts_agg a ON a.dance_id = d.id
LEFT JOIN views_agg v ON v.dance_id = d.id
LEFT JOIN segment_texts st ON st.dance_id = d.id
WHERE d.status = 'published'
ORDER BY d.id;
