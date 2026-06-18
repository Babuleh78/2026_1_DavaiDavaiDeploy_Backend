package repo

import _ "embed"

//go:embed sql/reels/get_reels_feed.sql
var GetReelsFeedQuery string

//go:embed sql/reels/get_reels_feed_count.sql
var GetReelsFeedCountQuery string

//go:embed sql/reels/get_user_reels_history.sql
var GetUserReelsHistoryQuery string

//go:embed sql/reels/get_candidate_dances_for_reels.sql
var GetCandidateDancesForReelsQuery string

//go:embed sql/reels/get_reels_by_ids.sql
var GetReelsByIDsQuery string
