package repo

import _ "embed"

//go:embed sql/getUserByIDQuery.sql
var GetUserByIDQuery string

//go:embed sql/getUserByLoginQuery.sql
var GetUserByLoginQuery string

//go:embed sql/updateUserPasswordQuery.sql
var UpdateUserPasswordQuery string

//go:embed sql/updateUserProfileQuery.sql
var UpdateUserProfileQuery string

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

//go:embed sql/saveRatingQuery.sql
var SaveRatingQuery string

//go:embed sql/getAggregatedRatingQuery.sql
var GetAggregatedRatingQuery string

//go:embed sql/createDanceQuery.sql
var CreateDanceQuery string

//go:embed sql/updateDanceStatusQuery.sql
var UpdateDanceStatusQuery string

//go:embed sql/getDanceStatusQuery.sql
var GetDanceStatusQuery string

//go:embed sql/recordDanceAttemptQuery.sql
var RecordDanceAttemptQuery string

//go:embed sql/getDanceCatalogCountQuery.sql
var GetDanceCatalogCountQuery string

//go:embed sql/getDanceStatsQuery.sql
var GetDanceStatsQuery string

//go:embed sql/getDancesEnrichedInfoQuery.sql
var GetDancesEnrichedInfoQuery string

//go:embed sql/getDanceTrendingQuery.sql
var GetDanceTrendingQuery string

//go:embed sql/getLeaderboardQuery.sql
var GetLeaderboardQuery string

//go:embed sql/getUserDanceRankQuery.sql
var GetUserDanceRankQuery string

//go:embed sql/getDanceVideoPathQuery.sql
var GetDanceVideoPathQuery string

//go:embed sql/saveAttemptQuery.sql
var SaveAttemptQuery string

//go:embed sql/unsaveAttemptQuery.sql
var UnsaveAttemptQuery string

//go:embed sql/getSavedAttemptsQuery.sql
var GetSavedAttemptsQuery string

//go:embed sql/getPersonalTopQuery.sql
var GetPersonalTopQuery string

//go:embed sql/getUserAttemptsQuery.sql
var GetUserAttemptsQuery string

//go:embed sql/getDanceProgressQuery.sql
var GetDanceProgressQuery string

//go:embed sql/getLastAttemptQuery.sql
var GetLastAttemptQuery string

//go:embed sql/getAttemptOwnerQuery.sql
var GetAttemptOwnerQuery string

//go:embed sql/isSavedAttemptWithVideoQuery.sql
var IsSavedAttemptWithVideoQuery string

//go:embed sql/isAttemptPrivateQuery.sql
var IsAttemptPrivateQuery string

//go:embed sql/savedAttemptExistsQuery.sql
var SavedAttemptExistsQuery string

//go:embed sql/recordDanceViewQuery.sql
var RecordDanceViewQuery string

//go:embed sql/linkDanceUploadQuery.sql
var LinkDanceUploadQuery string

//go:embed sql/getDanceUploadersQuery.sql
var GetDanceUploadersQuery string

//go:embed sql/getDanceAuthorQuery.sql
var GetDanceAuthorQuery string

//go:embed sql/setDanceModerationReasonQuery.sql
var SetDanceModerationReasonQuery string

//go:embed sql/getDanceModerationReasonQuery.sql
var GetDanceModerationReasonQuery string

//go:embed sql/createNotificationQuery.sql
var CreateNotificationQuery string

//go:embed sql/createFriendNotificationQuery.sql
var CreateFriendNotificationQuery string

//go:embed sql/getNotificationsQuery.sql
var GetNotificationsQuery string

//go:embed sql/markNotificationReadQuery.sql
var MarkNotificationReadQuery string

//go:embed sql/markAllNotificationsReadQuery.sql
var MarkAllNotificationsReadQuery string

//go:embed sql/clearNotificationsQuery.sql
var ClearNotificationsQuery string

//go:embed sql/createFriendshipQuery.sql
var CreateFriendshipQuery string

//go:embed sql/updateFriendshipStatusQuery.sql
var UpdateFriendshipStatusQuery string

//go:embed sql/getFriendsQuery.sql
var GetFriendsQuery string

//go:embed sql/searchUsersQuery.sql
var SearchUsersQuery string

//go:embed sql/getActiveDuelsForUserDanceQuery.sql
var GetActiveDuelsForUserDanceQuery string

//go:embed sql/hasOpenDuelQuery.sql
var HasOpenDuelQuery string

//go:embed sql/ensureSavedAttemptForDuelQuery.sql
var EnsureSavedAttemptForDuelQuery string

//go:embed sql/getFriendshipBetweenQuery.sql
var GetFriendshipBetweenQuery string

//go:embed sql/deleteFriendshipQuery.sql
var DeleteFriendshipQuery string

//go:embed sql/getFriendsCountQuery.sql
var GetFriendsCountQuery string

//go:embed sql/getUploadedDancesByUserQuery.sql
var GetUploadedDancesByUserQuery string

//go:embed sql/updateDanceTitleQuery.sql
var UpdateDanceTitleQuery string

//go:embed sql/updateDanceDifficultyQuery.sql
var UpdateDanceDifficultyQuery string

//go:embed sql/updateDanceDurationQuery.sql
var UpdateDanceDurationQuery string

//go:embed sql/createCompareTaskQuery.sql
var CreateCompareTaskQuery string

//go:embed sql/getCompareTaskQuery.sql
var GetCompareTaskQuery string

//go:embed sql/markCompareTaskFinalizedQuery.sql
var MarkCompareTaskFinalizedQuery string

//go:embed sql/getPublishedDanceIDsQuery.sql
var GetPublishedDanceIDsQuery string

//go:embed sql/createDuelNotificationQuery.sql
var CreateDuelNotificationQuery string

//go:embed sql/getBatchDuelParticipantsQuery.sql
var GetBatchDuelParticipantsQuery string

//go:embed sql/getDanceViewCountQuery.sql
var GetDanceViewCountQuery string

//go:embed sql/botFindUserQuery.sql
var BotFindUserQuery string

//go:embed sql/botUpdateTelegramIDQuery.sql
var BotUpdateTelegramIDQuery string

//go:embed sql/botGetUserByTelegramIDQuery.sql
var BotGetUserByTelegramIDQuery string

//go:embed sql/getUserTelegramIDQuery.sql
var GetUserTelegramIDQuery string

//go:embed sql/getUserActivityQuery.sql
var GetUserActivityQuery string

//go:embed sql/getMostImprovedDanceQuery.sql
var GetMostImprovedDanceQuery string

//go:embed sql/getCreatorDailyStatsQuery.sql
var GetCreatorDailyStatsQuery string

//go:embed sql/getCreatorTopDancesQuery.sql
var GetCreatorTopDancesQuery string

//go:embed sql/getReelsAttemptsQuery.sql
var GetReelsAttemptsQuery string

//go:embed sql/getUserGlobalRankQuery.sql
var GetUserGlobalRankQuery string

//go:embed sql/getFriendsScoresQuery.sql
var GetFriendsScoresQuery string

//go:embed sql/getFriendsFeedQuery.sql
var GetFriendsFeedQuery string

//go:embed sql/getAvgUnlockedAchievementsQuery.sql
var GetAvgUnlockedAchievementsQuery string

//go:embed sql/getDuelStatsQuery.sql
var GetDuelStatsQuery string

//go:embed sql/getUserWeakSpotsQuery.sql
var GetUserWeakSpotsQuery string
