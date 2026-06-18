SELECT
    COALESCE((
        SELECT COUNT(*) FROM dance_likes dl
        JOIN dance_uploads du ON du.dance_id = dl.dance_id
        WHERE du.user_id = $1
    ), 0)::bigint AS total_likes,

    COALESCE((
        SELECT MAX(score) FROM dance_attempts
        WHERE user_id = $1
    ), 0)::float AS max_score,

    COALESCE((
        SELECT COUNT(*) FROM dance_uploads
        WHERE user_id = $1
    ), 0)::bigint AS upload_count,

    COALESCE((
        SELECT COUNT(*) FROM dance_attempts
        WHERE user_id = $1 AND attempt_id IS NOT NULL
    ), 0)::bigint AS attempt_count,

    COALESCE((
        SELECT COUNT(*) FROM duels
        WHERE (challenger_id = $1 OR opponent_id = $1)
          AND status IN ('active', 'challenger_done', 'opponent_done', 'completed')
    ), 0)::bigint AS duel_count,

    COALESCE((
        SELECT COUNT(*) FROM duels
        WHERE winner_id = $1
          AND status = 'completed'
    ), 0)::bigint AS duel_win_count,

    COALESCE((
        SELECT COUNT(DISTINCT dance_id) FROM dance_attempts
        WHERE user_id = $1 AND attempt_id IS NOT NULL
    ), 0)::bigint AS unique_dance_count,

    COALESCE((
        SELECT MAX(streak_len)::bigint
        FROM (
            SELECT COUNT(*) AS streak_len
            FROM (
                SELECT
                    CASE WHEN winner_id = $1 THEN 1 ELSE 0 END AS is_win,
                    ROW_NUMBER() OVER (ORDER BY completed_at) -
                    ROW_NUMBER() OVER (PARTITION BY (CASE WHEN winner_id = $1 THEN 1 ELSE 0 END) ORDER BY completed_at) AS grp
                FROM duels
                WHERE (challenger_id = $1 OR opponent_id = $1)
                  AND status = 'completed'
            ) sub
            WHERE is_win = 1
            GROUP BY grp
        ) streaks
    ), 0)::bigint AS duel_win_streak;
