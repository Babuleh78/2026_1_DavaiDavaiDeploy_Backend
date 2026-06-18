UPDATE duels
SET status = 'expired'
WHERE expires_at < NOW()
  AND status IN ('pending', 'active', 'challenger_done', 'opponent_done')
RETURNING id;
