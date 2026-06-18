package pg

import _ "embed"

//go:embed sql/save_attempt.sql
var SaveAttemptQuery string

//go:embed sql/unsave_attempt.sql
var UnsaveAttemptQuery string

//go:embed sql/is_saved_attempt_with_video.sql
var IsSavedAttemptWithVideoQuery string

//go:embed sql/get_saved_attempts.sql
var GetSavedAttemptsQuery string

//go:embed sql/get_attempts_by_user.sql
var GetAttemptsByUserQuery string

//go:embed sql/get_attempt_owner.sql
var GetAttemptOwnerQuery string

//go:embed sql/get_dance_stats.sql
var GetDanceStatsQuery string

//go:embed sql/get_leaderboard.sql
var GetLeaderboardQuery string

//go:embed sql/get_user_dance_rank.sql
var GetUserDanceRankQuery string

//go:embed sql/record_dance_attempt.sql
var RecordDanceAttemptQuery string

//go:embed sql/get_top_dancers.sql
var GetTopDancersQuery string

//go:embed sql/get_user_reels_history.sql
var GetUserReelsHistoryQuery string

//go:embed sql/get_user_weak_spots.sql
var GetUserWeakSpotsQuery string
