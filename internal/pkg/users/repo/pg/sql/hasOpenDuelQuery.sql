SELECT EXISTS(
    SELECT 1
    FROM duels
    WHERE challenger_id = $1
      AND opponent_id = $2
      AND dance_id = $3
      AND status IN ('pending', 'active', 'challenger_done', 'opponent_done')
);
