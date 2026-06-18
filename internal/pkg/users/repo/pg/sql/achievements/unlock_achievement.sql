INSERT INTO user_achievements (user_id, achievement_id)
VALUES ($1, $2)
ON CONFLICT (user_id, achievement_id) DO NOTHING;
