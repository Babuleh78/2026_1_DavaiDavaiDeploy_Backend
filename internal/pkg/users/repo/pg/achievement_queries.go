package repo

import _ "embed"

//go:embed sql/achievements/get_all_achievements.sql
var GetAllAchievementsQuery string

//go:embed sql/achievements/get_user_achievements.sql
var GetUserAchievementsQuery string

//go:embed sql/achievements/unlock_achievement.sql
var UnlockAchievementQuery string

//go:embed sql/achievements/get_user_stats_for_achievements.sql
var GetUserStatsForAchievementsQuery string
