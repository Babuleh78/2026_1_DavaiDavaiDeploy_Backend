SELECT
    COUNT(*)::bigint                                                                         AS total,
    COUNT(*) FILTER (WHERE winner_id = $1)::bigint                                          AS wins,
    COALESCE(AVG(CASE WHEN challenger_id = $1 THEN challenger_score ELSE opponent_score END), 0)::float AS avg_score
FROM duels
WHERE (challenger_id = $1 OR opponent_id = $1)
  AND status = 'completed';
