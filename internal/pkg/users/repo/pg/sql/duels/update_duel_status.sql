UPDATE duels
SET
    status                    = $1,
    challenger_attempt_id     = $2,
    challenger_score          = $3,
    opponent_attempt_id       = $4,
    opponent_score            = $5,
    winner_id                 = $6,
    completed_at              = $7,
    -- Store per-user public consent when a participant submits ($9 IS NOT NULL).
    challenger_public_consent = CASE WHEN $9::boolean IS TRUE  THEN $10 ELSE challenger_public_consent END,
    opponent_public_consent   = CASE WHEN $9::boolean IS FALSE THEN $10 ELSE opponent_public_consent   END,
    -- Mark duel public when completing if both participants consented.
    is_public                 = CASE WHEN $1 = 'completed'
                                     THEN (CASE WHEN $9::boolean IS TRUE  THEN $10 ELSE challenger_public_consent END)
                                          AND (CASE WHEN $9::boolean IS FALSE THEN $10 ELSE opponent_public_consent END)
                                     ELSE is_public
                                END
WHERE id = $8
  AND status NOT IN ('completed', 'expired', 'declined')
  AND ($9::boolean IS NOT NULL OR status = 'pending')
  AND ($9::boolean IS NULL
       OR (CASE WHEN $9 THEN challenger_attempt_id IS NULL
                        ELSE opponent_attempt_id IS NULL
           END));
