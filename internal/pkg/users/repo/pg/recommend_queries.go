package repo

import _ "embed"

//go:embed sql/recommend/get_dances_for_recommender.sql
var GetDancesForRecommenderQuery string
