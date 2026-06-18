SELECT id, challenger_id, opponent_id
FROM duels
WHERE id = ANY($1::uuid[]);
