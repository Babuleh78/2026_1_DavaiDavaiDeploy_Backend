WITH ranked AS (
    SELECT
        DENSE_RANK() OVER (ORDER BY MAX(a.score) DESC)::int AS rank,
        u.login,
        MAX(a.score)::float AS score,
        a.user_id,
        u.avatar
    FROM dance_attempts a
    JOIN user_table u ON u.id = a.user_id
    WHERE a.dance_id = $1 AND a.user_id IS NOT NULL
    GROUP BY a.user_id, u.login, u.avatar
)
SELECT rank, login, score, avatar FROM ranked WHERE user_id = $2;
