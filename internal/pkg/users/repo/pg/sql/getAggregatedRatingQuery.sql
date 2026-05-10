SELECT
    COALESCE(AVG(physical), 0)::float       as avg_physical,
    COALESCE(AVG(speed), 0)::float          as avg_speed,
    COALESCE(AVG(coordination), 0)::float   as avg_coordination,
    COALESCE(AVG(repeatability), 0)::float  as avg_repeatability,
    COALESCE(AVG((physical + speed + coordination + repeatability) / 4.0), 0)::float as avg_score,
    COUNT(*)::int                           as total_ratings
FROM dance_ratings
WHERE video_id = $1;