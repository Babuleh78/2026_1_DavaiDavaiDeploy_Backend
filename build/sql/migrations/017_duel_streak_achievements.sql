INSERT INTO achievements (code, title, description, icon_key, category, threshold) VALUES
('duel_streak_3', 'Серия побед', 'Одержать 3 победы в дуэлях подряд', 'duel_streak_3', 'duel_streak', 3),
('duel_streak_5', 'Непобедимый', 'Одержать 5 побед в дуэлях подряд',  'duel_streak_5', 'duel_streak', 5)
ON CONFLICT (code) DO NOTHING;
