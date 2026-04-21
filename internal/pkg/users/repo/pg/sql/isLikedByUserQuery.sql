SELECT EXISTS(
    SELECT 1 FROM dance_likes
    WHERE user_id = $1 AND dance_id = $2
);