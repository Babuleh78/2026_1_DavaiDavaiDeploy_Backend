INSERT INTO notifications (user_id, type, duel_id, from_user_id)
VALUES ($1, $2, $3::uuid, $4);
