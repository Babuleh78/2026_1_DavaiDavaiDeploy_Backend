WITH inserted_duel AS (
    INSERT INTO duels (mode, challenger_id, opponent_id, dance_id, expires_at)
    VALUES ($1, $2, $3, NULLIF($4, ''), $5)
    RETURNING id
)
INSERT INTO duel_invites (duel_id, token)
SELECT id, $6 FROM inserted_duel
RETURNING duel_id, token;
