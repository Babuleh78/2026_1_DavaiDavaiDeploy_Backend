SELECT id FROM dances WHERE id = ANY($1) AND status = 'published';
