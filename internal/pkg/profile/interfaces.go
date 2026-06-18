package profile

import (
	"DDDance/internal/models"
	"context"

	uuid "github.com/satori/go.uuid"
)

type ProfileUsecase interface {
	UpdateProfile(ctx context.Context, id uuid.UUID, newLogin *string, avatarBuffer []byte, avatarContentType string) (models.User, string, error)
	GetPublicProfile(ctx context.Context, profileUserID uuid.UUID, viewerUserID *uuid.UUID) (*models.PublicProfileResponse, error)
	GetHistory(ctx context.Context, userID uuid.UUID) ([]models.SearchHistoryItem, error)
	DeleteFromHistory(ctx context.Context, historyID uuid.UUID, userID uuid.UUID) error
	UpdateHistoryName(ctx context.Context, historyID uuid.UUID, userID uuid.UUID, name string) error
	SendFriendRequest(ctx context.Context, senderID, receiverID uuid.UUID) error
	RespondFriendRequest(ctx context.Context, userID uuid.UUID, friendshipID int64, accept bool) error
	GetFriends(ctx context.Context, userID uuid.UUID) ([]models.Friend, error)
	SearchUsers(ctx context.Context, query string) ([]models.UserSearchItem, error)
	RemoveFriend(ctx context.Context, userID, friendID uuid.UUID) error
	GetUserActivity(ctx context.Context, userID uuid.UUID) ([]models.ActivityEntry, error)
	GetMostImprovedDance(ctx context.Context, userID uuid.UUID) (*models.MostImprovedDance, error)
	GetCreatorAnalytics(ctx context.Context, userID uuid.UUID) (*models.CreatorAnalytics, error)
}

type ProfileRepo interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error)
	UpdateUserProfile(ctx context.Context, userID uuid.UUID, login *string, avatar *string, bumpVersion bool) error
	GetHistory(ctx context.Context, userID uuid.UUID) ([]models.SearchHistoryItem, error)
	DeleteFromHistory(ctx context.Context, historyID uuid.UUID, userID uuid.UUID) error
	UpdateHistoryName(ctx context.Context, historyID uuid.UUID, userID uuid.UUID, name string) error
	GetSavedAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.SavedAttemptItem, error)
	GetUploadedDancesByUser(ctx context.Context, userID uuid.UUID) ([]models.UploadedDance, error)
	GetPersonalTop(ctx context.Context, userID uuid.UUID) ([]models.PersonalTopItem, error)
	GetFriendsCount(ctx context.Context, userID uuid.UUID) (int, error)
	GetFriendshipBetween(ctx context.Context, userID, otherID uuid.UUID) (*models.FriendshipStatus, error)
	CreateFriendship(ctx context.Context, senderID, receiverID uuid.UUID) (int64, error)
	UpdateFriendshipStatus(ctx context.Context, friendshipID int64, receiverID uuid.UUID, status string) (uuid.UUID, error)
	GetFriends(ctx context.Context, userID uuid.UUID) ([]models.Friend, error)
	SearchUsers(ctx context.Context, query string, limit int) ([]models.UserSearchItem, error)
	DeleteFriendship(ctx context.Context, userID, friendID uuid.UUID) error
	CreateFriendNotification(ctx context.Context, toUserID, fromUserID uuid.UUID, notifType string, friendshipID int64) error
	GetUserStatsForAchievements(ctx context.Context, userID uuid.UUID) (*models.UserAchievementStats, error)
	GetUserActivity(ctx context.Context, userID uuid.UUID) ([]models.ActivityEntry, error)
	GetMostImprovedDance(ctx context.Context, userID uuid.UUID) (*models.MostImprovedDance, error)
	GetCreatorAnalytics(ctx context.Context, userID uuid.UUID) (*models.CreatorAnalytics, error)
}

type ProfileStorageRepo interface {
	UploadAvatar(ctx context.Context, buffer []byte, contentType string, userID string) (string, error)
}

type TokenGenerator interface {
	GenerateToken(id uuid.UUID, login string, version int) (string, error)
}
