package repo

import _ "embed"

//go:embed sql/top/get_top_dancers.sql
var GetTopDancersQuery string

//go:embed sql/top/get_top_dances.sql
var GetTopDancesQuery string

//go:embed sql/top/get_top_dancers_by_ids.sql
var GetTopDancersByIDsQuery string
