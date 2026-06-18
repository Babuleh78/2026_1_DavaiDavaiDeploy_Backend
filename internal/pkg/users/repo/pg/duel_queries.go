package repo

import _ "embed"

//go:embed sql/duels/create_duel.sql
var CreateDuelQuery string

//go:embed sql/duels/get_duel_by_id.sql
var GetDuelByIDQuery string

//go:embed sql/duels/get_duels_by_user.sql
var GetDuelsByUserQuery string

//go:embed sql/duels/update_duel_status.sql
var UpdateDuelStatusQuery string

//go:embed sql/duels/expire_duels.sql
var ExpireDuelsQuery string

//go:embed sql/duels/get_random_dance.sql
var GetRandomDanceQuery string

//go:embed sql/duels/get_distinct_challengers_count.sql
var GetDistinctChallengersCountQuery string

//go:embed sql/duels/get_public_duels.sql
var GetPublicDuelsQuery string
