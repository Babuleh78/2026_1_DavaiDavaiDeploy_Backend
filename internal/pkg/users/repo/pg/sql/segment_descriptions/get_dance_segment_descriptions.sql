SELECT segment_index, description
FROM segment_descriptions
WHERE dance_id = $1
ORDER BY segment_index;
