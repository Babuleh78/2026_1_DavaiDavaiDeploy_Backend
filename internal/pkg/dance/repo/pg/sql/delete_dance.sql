-- Delete a dance and all related records in dependency order.
-- Steps are executed sequentially in Go (DeleteDanceAndRelated).
DELETE FROM dances WHERE id = $1;
