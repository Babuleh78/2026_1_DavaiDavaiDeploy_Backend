package usecase

import (
	"testing"

	"DDDance/internal/models"
)

// TestMeetsThreshold exercises every achievement category branch, asserting the
// at-threshold (>=) boundary and the just-below case map to true/false.
func TestMeetsThreshold(t *testing.T) {
	cases := []struct {
		name  string
		ach   models.Achievement
		stats models.UserAchievementStats
		want  bool
	}{
		{
			name:  "likes at threshold",
			ach:   models.Achievement{Category: "likes", Threshold: 10},
			stats: models.UserAchievementStats{TotalLikes: 10},
			want:  true,
		},
		{
			name:  "likes below threshold",
			ach:   models.Achievement{Category: "likes", Threshold: 10},
			stats: models.UserAchievementStats{TotalLikes: 9},
			want:  false,
		},
		{
			name:  "score at threshold",
			ach:   models.Achievement{Category: "score", Threshold: 90},
			stats: models.UserAchievementStats{MaxScore: 90},
			want:  true,
		},
		{
			name:  "score below threshold",
			ach:   models.Achievement{Category: "score", Threshold: 90},
			stats: models.UserAchievementStats{MaxScore: 89.9},
			want:  false,
		},
		{
			name:  "upload at threshold",
			ach:   models.Achievement{Category: "upload", Threshold: 5},
			stats: models.UserAchievementStats{UploadCount: 5},
			want:  true,
		},
		{
			name:  "attempt below threshold",
			ach:   models.Achievement{Category: "attempt", Threshold: 100},
			stats: models.UserAchievementStats{AttemptCount: 50},
			want:  false,
		},
		{
			name:  "duel at threshold",
			ach:   models.Achievement{Category: "duel", Threshold: 3},
			stats: models.UserAchievementStats{DuelCount: 3},
			want:  true,
		},
		{
			name:  "duel_win at threshold",
			ach:   models.Achievement{Category: "duel_win", Threshold: 7},
			stats: models.UserAchievementStats{DuelWinCount: 8},
			want:  true,
		},
		{
			name:  "duel_streak below threshold",
			ach:   models.Achievement{Category: "duel_streak", Threshold: 5},
			stats: models.UserAchievementStats{DuelWinStreak: 4},
			want:  false,
		},
		{
			name:  "special variety_dancer at threshold",
			ach:   models.Achievement{Category: "special", Code: "variety_dancer", Threshold: 6},
			stats: models.UserAchievementStats{UniqueDanceCount: 6},
			want:  true,
		},
		{
			name:  "special unknown code never unlocks",
			ach:   models.Achievement{Category: "special", Code: "mystery", Threshold: 1},
			stats: models.UserAchievementStats{UniqueDanceCount: 999},
			want:  false,
		},
		{
			name:  "unknown category never unlocks",
			ach:   models.Achievement{Category: "bogus", Threshold: 0},
			stats: models.UserAchievementStats{},
			want:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := meetsThreshold(tc.ach, &tc.stats); got != tc.want {
				t.Errorf("meetsThreshold(%s, threshold=%d) = %v, want %v",
					tc.ach.Category, tc.ach.Threshold, got, tc.want)
			}
		})
	}
}
