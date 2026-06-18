CREATE TABLE IF NOT EXISTS achievements (
    id          SERIAL PRIMARY KEY,
    code        TEXT UNIQUE NOT NULL,
    title       TEXT NOT NULL,
    description TEXT NOT NULL,
    icon_key    TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL,
    threshold   INT  NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS user_achievements (
    user_id        uuid NOT NULL REFERENCES user_table(id) ON DELETE CASCADE,
    achievement_id INT  NOT NULL REFERENCES achievements(id) ON DELETE CASCADE,
    unlocked_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, achievement_id)
);

CREATE INDEX IF NOT EXISTS user_achievements_user_id_idx ON user_achievements(user_id);

INSERT INTO achievements (code, title, description, icon_key, category, threshold) VALUES
('first_blood',      'Первая кровь',         'Получить 1 лайк на свой танец',                 'first_blood',      'likes',    1),
('crowd_pleaser',    'Нравится толпе',        'Получить 10 лайков на танцы',                   'crowd_pleaser',    'likes',    10),
('fan_favorite',     'Любимчик фанатов',      'Получить 50 лайков на танцы',                   'fan_favorite',     'likes',    50),
('half_score',       'Неплохо',               'Получить скор выше 50 в сравнении',             'half_score',       'score',    50),
('almost_perfect',   'Почти идеально',        'Получить скор выше 70 в сравнении',             'almost_perfect',   'score',    70),
('perfectionist',    'Перфекционист',         'Получить скор выше 90 в сравнении',             'perfectionist',    'score',    90),
('first_upload',     'Первый шаг',            'Загрузить первый танец',                        'first_upload',     'upload',   1),
('choreographer',    'Хореограф',             'Загрузить 5 танцев',                            'choreographer',    'upload',   5),
('dance_library',    'Библиотека движений',   'Загрузить 20 танцев',                           'dance_library',    'upload',   20),
('just_try',         'Просто попробуй',       'Сделать первую попытку',                        'just_try',         'attempt',  1),
('persistent',       'Настойчивый',           'Сделать 10 попыток',                            'persistent',       'attempt',  10),
('grinder',          'Шлифовщик',             'Сделать 50 попыток',                            'grinder',          'attempt',  50),
('first_duel',       'Первая дуэль',          'Принять участие в дуэли',                       'first_duel',       'duel',     1),
('duel_winner',      'Дуэлянт',               'Победить в 3 дуэлях',                           'duel_winner',      'duel_win', 3),
('duel_champion',    'Чемпион дуэлей',        'Победить в 10 дуэлях',                          'duel_champion',    'duel_win', 10),
('night_dancer',     'Ночной танцор',         'Загрузить попытку между 00:00 и 05:00',         'night_dancer',     'special',  1),
('speed_learner',    'Быстрый ученик',        'Пройти танец за один день после загрузки',      'speed_learner',    'special',  1),
('variety_dancer',   'Разносторонний',        'Попробовать 5 разных танцев',                   'variety_dancer',   'special',  5),
('social_butterfly', 'Социальная бабочка',    'Получить вызов на дуэль от 3 разных игроков',  'social_butterfly', 'special',  3),
('top_dancer',       'Топ танцор',            'Войти в топ-10 сайта',                          'top_dancer',       'special',  10)
ON CONFLICT (code) DO NOTHING;
