SELECT description
FROM segment_descriptions
WHERE dance_id = $1
  AND segment_index = $2;
