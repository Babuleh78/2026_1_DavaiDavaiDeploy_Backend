SELECT d.id,
       CASE WHEN d.challenger_id = $1 THEN ou.login ELSE cu.login END AS other_login
FROM duels d
JOIN user_table cu ON cu.id = d.challenger_id
JOIN user_table ou ON ou.id = d.opponent_id
WHERE d.dance_id = $2
  AND d.status IN ('active', 'challenger_done', 'opponent_done')
  AND d.expires_at > NOW()
  AND (
      (d.challenger_id = $1 AND d.challenger_attempt_id IS NULL)
   OR (d.opponent_id = $1 AND d.opponent_attempt_id IS NULL)
  )
ORDER BY d.created_at;
