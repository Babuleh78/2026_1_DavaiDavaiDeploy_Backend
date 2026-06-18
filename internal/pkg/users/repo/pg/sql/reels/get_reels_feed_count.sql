SELECT COUNT(*)
FROM dances d
JOIN dance_uploads du ON du.dance_id = d.id
WHERE d.status = 'published'
  AND ($1::text[] IS NULL OR d.id != ALL($1::text[]));
