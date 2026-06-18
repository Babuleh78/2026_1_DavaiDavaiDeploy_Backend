INSERT INTO segment_descriptions (dance_id, segment_index, description, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (dance_id, segment_index)
DO UPDATE SET
    description = EXCLUDED.description,
    updated_at  = NOW();
