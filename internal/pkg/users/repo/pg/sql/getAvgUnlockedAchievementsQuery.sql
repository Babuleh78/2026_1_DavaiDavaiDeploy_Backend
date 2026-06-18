SELECT COALESCE(AVG(unlocked_count), 0)::float
FROM (
    SELECT user_id, COUNT(*) AS unlocked_count
    FROM user_achievements
    GROUP BY user_id
) t;
