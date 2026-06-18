SELECT a.id, a.code, a.title, a.description, a.icon_key, a.category, a.threshold, ua.unlocked_at
FROM user_achievements ua
JOIN achievements a ON a.id = ua.achievement_id
WHERE ua.user_id = $1
ORDER BY ua.unlocked_at DESC;
