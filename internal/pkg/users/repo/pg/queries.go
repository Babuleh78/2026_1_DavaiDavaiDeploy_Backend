package repo

import _ "embed"

//go:embed sql/getUserByIDQuery.sql
var GetUserByIDQuery string

//go:embed sql/getUserByLoginQuery.sql
var GetUserByLoginQuery string

//go:embed sql/updateUserPasswordQuery.sql
var UpdateUserPasswordQuery string

//go:embed sql/addToHistoryQuery.sql
var AddToHistoryQuery string

//go:embed sql/getHistoryQuery.sql
var GetHistoryQuery string

//go:embed sql/deleteFromHistoryQuery.sql
var DeleteFromHistoryQuery string

//go:embed sql/updateHistoryNameQuery.sql
var UpdateHistoryNameQuery string

//go:embed sql/toggleLikeQuery.sql
var ToggleLikeQuery string

//go:embed sql/getLikesCountQuery.sql
var GetLikesCountQuery string

//go:embed sql/isLikedByUserQuery.sql
var IsLikedByUserQuery string

//go:embed sql/getTopLikedDancesQuery.sql
var GetTopLikedDancesQuery string

//go:embed sql/deleteLikeQuery.sql
var DeleteLikeQuery string

//go:embed sql/getUserLikedDancesQuery.sql
var GetUserLikedDancesQuery string

//go:embed sql/cleanHistoryQuery.sql
var CleanHistoryQuery string