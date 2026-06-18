SELECT COUNT(DISTINCT challenger_id)
FROM duels
WHERE opponent_id = $1;
