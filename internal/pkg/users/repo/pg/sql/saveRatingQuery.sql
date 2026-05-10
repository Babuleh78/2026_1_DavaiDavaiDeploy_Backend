INSERT INTO dance_ratings (video_id, user_id, physical, speed, coordination, repeatability)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (video_id, user_id) DO UPDATE SET
    physical = EXCLUDED.physical,
    speed = EXCLUDED.speed,
    coordination = EXCLUDED.coordination,
    repeatability = EXCLUDED.repeatability,
    created_at = CURRENT_TIMESTAMP;