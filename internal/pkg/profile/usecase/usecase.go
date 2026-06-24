package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/profile"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"log/slog"
	"strings"

	uuid "github.com/satori/go.uuid"
)

type ProfileUsecase struct {
	profileRepo    profile.ProfileRepo
	storageRepo    profile.ProfileStorageRepo
	tokenGenerator profile.TokenGenerator
}

// NewProfileUsecase wires the profile usecase. The token generator is a
// required collaborator (used to re-issue tokens after a login change) and is
// passed at construction.
func NewProfileUsecase(profileRepo profile.ProfileRepo, storageRepo profile.ProfileStorageRepo, tokenGenerator profile.TokenGenerator) *ProfileUsecase {
	return &ProfileUsecase{
		profileRepo:    profileRepo,
		storageRepo:    storageRepo,
		tokenGenerator: tokenGenerator,
	}
}

func (uc *ProfileUsecase) UpdateProfile(ctx context.Context, id uuid.UUID, newLogin *string, avatarBuffer []byte, avatarContentType string) (models.User, string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	current, err := uc.profileRepo.GetUserByID(ctx, id)
	if err != nil {
		return models.User{}, "", err
	}

	var loginPtr *string
	bumpVersion := false
	if newLogin != nil {
		trimmed := strings.TrimSpace(*newLogin)
		if trimmed != current.Login {
			if !isValidLogin(trimmed) {
				logger.Error("invalid login format")
				return models.User{}, "", users.ErrorBadRequest
			}
			loginPtr = &trimmed
			bumpVersion = true
		}
	}

	var avatarPtr *string
	if len(avatarBuffer) > 0 {
		if avatarContentType != "image/jpeg" && avatarContentType != "image/png" && avatarContentType != "image/webp" {
			logger.Error("invalid avatar content type", "ct", avatarContentType)
			return models.User{}, "", users.ErrorBadRequest
		}
		key, err := uc.storageRepo.UploadAvatar(ctx, avatarBuffer, avatarContentType, id.String())
		if err != nil {
			logger.Error("failed to upload avatar", "error", err)
			return models.User{}, "", users.ErrorInternalServerError
		}
		avatarPtr = &key
	}

	if loginPtr == nil && avatarPtr == nil {
		return current, "", nil
	}

	if err := uc.profileRepo.UpdateUserProfile(ctx, id, loginPtr, avatarPtr, bumpVersion); err != nil {
		return models.User{}, "", err
	}

	updated, err := uc.profileRepo.GetUserByID(ctx, id)
	if err != nil {
		return models.User{}, "", err
	}

	var newToken string
	if bumpVersion && uc.tokenGenerator != nil {
		newToken, err = uc.tokenGenerator.GenerateToken(updated.ID, updated.Login, updated.Version)
		if err != nil {
			return models.User{}, "", err
		}
	}

	return updated, newToken, nil
}

func (uc *ProfileUsecase) GetPublicProfile(ctx context.Context, profileUserID uuid.UUID, viewerUserID *uuid.UUID) (*models.PublicProfileResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	user, err := uc.profileRepo.GetUserByID(ctx, profileUserID)
	if err != nil {
		return nil, err
	}

	saved, err := uc.profileRepo.GetSavedAttempts(ctx, profileUserID, 50, 0)
	if err != nil {
		logger.Warn("failed to load saved attempts", "error", err)
		saved = []models.SavedAttemptItem{}
	}

	top, err := uc.profileRepo.GetPersonalTop(ctx, profileUserID)
	if err != nil {
		logger.Warn("failed to load personal top", "error", err)
		top = []models.PersonalTopItem{}
	}

	isOwn := viewerUserID != nil && *viewerUserID == profileUserID

	if !isOwn {
		filtered := saved[:0]
		for _, s := range saved {
			if !s.IsPrivate {
				filtered = append(filtered, s)
			}
		}
		saved = filtered
	}

	var uploaded []models.UploadedDance
	if !isOwn {
		if ups, upErr := uc.profileRepo.GetUploadedDancesByUser(ctx, profileUserID); upErr == nil {
			for _, d := range ups {
				if d.Status == "published" {
					uploaded = append(uploaded, d)
				}
			}
		} else {
			logger.Warn("failed to load uploaded dances", "error", upErr)
		}
	}

	friendsCount, _ := uc.profileRepo.GetFriendsCount(ctx, profileUserID)

	var friendshipStatus *models.FriendshipStatus
	if viewerUserID != nil && !isOwn {
		friendshipStatus, _ = uc.profileRepo.GetFriendshipBetween(ctx, *viewerUserID, profileUserID)
	}

	var stats *models.ProfileStats
	if rawStats, statsErr := uc.profileRepo.GetUserStatsForAchievements(ctx, profileUserID); statsErr == nil {
		stats = &models.ProfileStats{
			AttemptCount:     rawStats.AttemptCount,
			MaxScore:         rawStats.MaxScore,
			UniqueDanceCount: rawStats.UniqueDanceCount,
			DuelWinCount:     rawStats.DuelWinCount,
			UploadCount:      rawStats.UploadCount,
		}
	} else {
		logger.Warn("failed to load profile stats", "error", statsErr)
	}

	return &models.PublicProfileResponse{
		User: models.PublicProfileUser{
			ID:        user.ID,
			Login:     user.Login,
			Avatar:    user.Avatar,
			UpdatedAt: user.UpdatedAt,
		},
		SavedAttempts:    saved,
		UploadedDances:   uploaded,
		PersonalTop:      top,
		IsOwnProfile:     isOwn,
		FriendsCount:     friendsCount,
		FriendshipStatus: friendshipStatus,
		Stats:            stats,
	}, nil
}

func (uc *ProfileUsecase) GetHistory(ctx context.Context, userID uuid.UUID) ([]models.SearchHistoryItem, error) {
	return uc.profileRepo.GetHistory(ctx, userID)
}

func (uc *ProfileUsecase) DeleteFromHistory(ctx context.Context, historyID uuid.UUID, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if historyID == uuid.Nil || userID == uuid.Nil {
		logger.Error("invalid historyID or userID")
		return users.ErrorBadRequest
	}

	err := uc.profileRepo.DeleteFromHistory(ctx, historyID, userID)
	if err != nil {
		logger.Error("failed to delete from history", "error", err)
		return err
	}

	logger.Info("successfully deleted history item")
	return nil
}

func (uc *ProfileUsecase) UpdateHistoryName(ctx context.Context, historyID uuid.UUID, userID uuid.UUID, name string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if historyID == uuid.Nil || userID == uuid.Nil {
		logger.Error("invalid historyID or userID")
		return users.ErrorBadRequest
	}

	name = strings.TrimSpace(name)
	if name == "" {
		logger.Error("name is empty")
		return users.ErrorBadRequest
	}
	if len(name) > 100 {
		logger.Error("name is too long")
		return users.ErrorBadRequest
	}

	err := uc.profileRepo.UpdateHistoryName(ctx, historyID, userID, name)
	if err != nil {
		logger.Error("failed to update history name", "error", err)
		return err
	}

	logger.Info("successfully updated history name")
	return nil
}

func (uc *ProfileUsecase) SendFriendRequest(ctx context.Context, senderID, receiverID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if senderID == receiverID {
		return users.ErrorBadRequest
	}

	if _, err := uc.profileRepo.GetUserByID(ctx, receiverID); err != nil {
		return users.ErrorNotFound
	}
	friendshipID, err := uc.profileRepo.CreateFriendship(ctx, senderID, receiverID)
	if err != nil {
		return err
	}
	if notifErr := uc.profileRepo.CreateFriendNotification(ctx, receiverID, senderID, "friend_request", friendshipID); notifErr != nil {
		logger.Warn("failed to create friend_request notification", "error", notifErr)
	}
	return nil
}

func (uc *ProfileUsecase) RespondFriendRequest(ctx context.Context, userID uuid.UUID, friendshipID int64, accept bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	status := "declined"
	if accept {
		status = "accepted"
	}
	senderID, err := uc.profileRepo.UpdateFriendshipStatus(ctx, friendshipID, userID, status)
	if err != nil {
		return err
	}
	notifType := "friend_declined"
	if accept {
		notifType = "friend_accepted"
	}
	if notifErr := uc.profileRepo.CreateFriendNotification(ctx, senderID, userID, notifType, friendshipID); notifErr != nil {
		logger.Warn("failed to create friend response notification", "error", notifErr)
	}
	return nil
}

func (uc *ProfileUsecase) GetFriends(ctx context.Context, userID uuid.UUID) ([]models.Friend, error) {
	return uc.profileRepo.GetFriends(ctx, userID)
}

const searchUsersLimit = 20

func (uc *ProfileUsecase) SearchUsers(ctx context.Context, query string) ([]models.UserSearchItem, error) {
	return uc.profileRepo.SearchUsers(ctx, query, searchUsersLimit)
}

func (uc *ProfileUsecase) RemoveFriend(ctx context.Context, userID, friendID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if userID == friendID {
		return users.ErrorBadRequest
	}

	var friendshipID int64
	if fs, fsErr := uc.profileRepo.GetFriendshipBetween(ctx, userID, friendID); fsErr == nil && fs != nil {
		friendshipID = fs.FriendshipID
	}
	if err := uc.profileRepo.DeleteFriendship(ctx, userID, friendID); err != nil {
		return err
	}

	if notifErr := uc.profileRepo.CreateFriendNotification(ctx, friendID, userID, "friend_removed", friendshipID); notifErr != nil {
		logger.Warn("failed to create friend_removed notification", "error", notifErr)
	}
	return nil
}

func (uc *ProfileUsecase) GetUserActivity(ctx context.Context, userID uuid.UUID) ([]models.ActivityEntry, error) {
	return uc.profileRepo.GetUserActivity(ctx, userID)
}

func (uc *ProfileUsecase) GetMostImprovedDance(ctx context.Context, userID uuid.UUID) (*models.MostImprovedDance, error) {
	return uc.profileRepo.GetMostImprovedDance(ctx, userID)
}

func (uc *ProfileUsecase) GetCreatorAnalytics(ctx context.Context, userID uuid.UUID) (*models.CreatorAnalytics, error) {
	return uc.profileRepo.GetCreatorAnalytics(ctx, userID)
}

func isValidLogin(login string) bool {
	if len(login) < 6 || len(login) > 15 {
		return false
	}
	for _, char := range login {
		if !strings.ContainsRune(users.ValidChars, char) {
			return false
		}
	}
	return true
}
