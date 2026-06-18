SELECT
    d.id,
    d.mode,
    d.challenger_id,
    d.opponent_id,
    COALESCE(d.dance_id, '')           AS dance_id,
    d.status,
    d.challenger_attempt_id,
    d.opponent_attempt_id,
    d.challenger_score,
    d.opponent_score,
    d.winner_id,
    d.expires_at,
    d.created_at,
    d.completed_at,
    d.is_public,
    c.login                            AS challenger_login,
    COALESCE(c.avatar, '')             AS challenger_avatar,
    o.login                            AS opponent_login,
    COALESCE(o.avatar, '')             AS opponent_avatar,
    COALESCE(dn.title, '')             AS dance_title,
    COALESCE(di.token, '')             AS invite_token
FROM duels d
JOIN user_table c  ON c.id = d.challenger_id
JOIN user_table o  ON o.id = d.opponent_id
LEFT JOIN dances dn ON dn.id = d.dance_id
LEFT JOIN duel_invites di ON di.duel_id = d.id
WHERE d.id = $1;
