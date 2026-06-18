package pg

import _ "embed"

//go:embed sql/upload_dance.sql
var uploadDanceQuery string

//go:embed sql/get_dance.sql
var getDanceQuery string

//go:embed sql/get_dances.sql
var getDancesQuery string

//go:embed sql/delete_dance.sql
var deleteDanceQuery string

//go:embed sql/get_top_dances.sql
var getTopDancesQuery string

//go:embed sql/get_reels_feed.sql
var getReelsFeedQuery string

//go:embed sql/get_candidate_dances_for_reels.sql
var getCandidateDancesForReelsQuery string
