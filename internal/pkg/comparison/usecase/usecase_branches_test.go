package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"DDDance/internal/models"
	"DDDance/internal/pkg/comparison"

	uuid "github.com/satori/go.uuid"
)

// s3NotFound mimics the error shape isS3NotFoundError recognises.
var s3NotFound = errors.New("NoSuchKey: the specified key does not exist")

// stubRepo embeds ComparisonRepo so only the methods a given test exercises need
// bodies. Behaviour is driven by func fields; unset funcs return zero values.
type stubRepo struct {
	comparison.ComparisonRepo

	owner       *uuid.UUID
	ownerErr    error
	isPrivate   bool
	danceLB     []models.LeaderboardEntry
	danceLBErr  error
	userRank    *models.LeaderboardEntry
	userRankErr error

	hadVideo     bool
	hadVideoErr  error
	unsaveErr    error
	unsaveCalled bool
	unsavedID    string
}

func (s *stubRepo) GetAttemptOwner(_ context.Context, _ string) (*uuid.UUID, error) {
	return s.owner, s.ownerErr
}
func (s *stubRepo) IsAttemptPrivate(_ context.Context, _ string) (bool, error) {
	return s.isPrivate, nil
}
func (s *stubRepo) GetDanceStats(_ context.Context, _ string) (*models.DanceStats, error) {
	return &models.DanceStats{AttemptCount: 3, TopScore: 88}, nil
}
func (s *stubRepo) GetUserByID(_ context.Context, id uuid.UUID) (models.User, error) {
	return models.User{ID: id, Login: "owner-login"}, nil
}
func (s *stubRepo) GetDanceLeaderboard(_ context.Context, _ string) ([]models.LeaderboardEntry, error) {
	return s.danceLB, s.danceLBErr
}
func (s *stubRepo) GetUserDanceRank(_ context.Context, _ string, _ uuid.UUID) (*models.LeaderboardEntry, error) {
	return s.userRank, s.userRankErr
}
func (s *stubRepo) IsSavedAttemptWithVideo(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
	return s.hadVideo, s.hadVideoErr
}
func (s *stubRepo) UnsaveAttempt(_ context.Context, _ uuid.UUID, attemptID string) error {
	s.unsaveCalled = true
	s.unsavedID = attemptID
	return s.unsaveErr
}

// stubStore serves DownloadFile from an in-memory map, FileExists from a set,
// and records DeleteFile calls.
type stubStore struct {
	comparison.ComparisonStorageRepo

	files   map[string][]byte
	exists  map[string]bool
	deleted []string
}

func (s *stubStore) DownloadFile(_ context.Context, key string) ([]byte, error) {
	if d, ok := s.files[key]; ok {
		return d, nil
	}
	return nil, s3NotFound
}
func (s *stubStore) FileExists(_ context.Context, key string) bool { return s.exists[key] }
func (s *stubStore) DeleteFile(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

func resultJSON(t *testing.T, r models.CompareStatusResult) []byte {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	return b
}

// --- GetCompareResult: three-path resolution (fragile area per CLAUDE.md) ---

func TestGetCompareResult_OwnerPathPreferred(t *testing.T) {
	viewer := uuid.NewV4()
	owner := viewer // viewer is the owner
	attempt := uuid.NewV4().String()

	ownerKey := "users/" + owner.String() + "/" + attempt + "/comparison_result.json"
	store := &stubStore{files: map[string][]byte{
		ownerKey: resultJSON(t, models.CompareStatusResult{DanceID: "dance-1", ComparisonScore: 73}),
	}}
	repo := &stubRepo{owner: &owner}
	uc := NewComparisonUsecase(repo, store, nil, nil, nil)

	res, err := uc.GetCompareResult(context.Background(), viewer, attempt)
	if err != nil {
		t.Fatalf("GetCompareResult: %v", err)
	}
	if res.Score != 73 || res.DanceID != "dance-1" {
		t.Errorf("got score=%v dance=%q, want 73/dance-1", res.Score, res.DanceID)
	}
	// s3UserID comes from the resolved key → skeleton key must use the owner ID.
	wantSkeleton := "users/" + owner.String() + "/" + attempt + "/skeleton.json"
	if res.UserSkeletonKey != wantSkeleton {
		t.Errorf("UserSkeletonKey = %q, want %q", res.UserSkeletonKey, wantSkeleton)
	}
}

func TestGetCompareResult_AnonPathFallback(t *testing.T) {
	viewer := uuid.NewV4()
	attempt := uuid.NewV4().String()

	// No owner known; only the anon layout (attempt/attempt) exists.
	anonKey := "users/" + attempt + "/" + attempt + "/comparison_result.json"
	store := &stubStore{files: map[string][]byte{
		anonKey: resultJSON(t, models.CompareStatusResult{DanceID: "dance-2", ComparisonScore: 50}),
	}}
	repo := &stubRepo{owner: nil}
	uc := NewComparisonUsecase(repo, store, nil, nil, nil)

	res, err := uc.GetCompareResult(context.Background(), viewer, attempt)
	if err != nil {
		t.Fatalf("GetCompareResult: %v", err)
	}
	if res.Score != 50 {
		t.Errorf("score = %v, want 50 (anon path)", res.Score)
	}
}

func TestGetCompareResult_PrivateAttemptForbiddenForNonOwner(t *testing.T) {
	viewer := uuid.NewV4()
	owner := uuid.NewV4() // different person
	attempt := uuid.NewV4().String()

	repo := &stubRepo{owner: &owner, isPrivate: true}
	store := &stubStore{files: map[string][]byte{}}
	uc := NewComparisonUsecase(repo, store, nil, nil, nil)

	_, err := uc.GetCompareResult(context.Background(), viewer, attempt)
	if !errors.Is(err, comparison.ErrorForbidden) {
		t.Errorf("err = %v, want ErrorForbidden for private attempt viewed by non-owner", err)
	}
}

func TestGetCompareResult_NotFoundWhenNoCandidateExists(t *testing.T) {
	viewer := uuid.NewV4()
	attempt := uuid.NewV4().String()

	repo := &stubRepo{owner: nil}
	store := &stubStore{files: map[string][]byte{}} // every DownloadFile → NoSuchKey
	uc := NewComparisonUsecase(repo, store, nil, nil, nil)

	_, err := uc.GetCompareResult(context.Background(), viewer, attempt)
	if !errors.Is(err, comparison.ErrorNotFound) {
		t.Errorf("err = %v, want ErrorNotFound", err)
	}
}

func TestGetCompareResult_OwnerInfoAttachedForOtherViewer(t *testing.T) {
	viewer := uuid.NewV4()
	owner := uuid.NewV4()
	attempt := uuid.NewV4().String()

	ownerKey := "users/" + owner.String() + "/" + attempt + "/comparison_result.json"
	store := &stubStore{files: map[string][]byte{
		ownerKey: resultJSON(t, models.CompareStatusResult{DanceID: "d", ComparisonScore: 60}),
	}}
	repo := &stubRepo{owner: &owner, isPrivate: false}
	uc := NewComparisonUsecase(repo, store, nil, nil, nil)

	res, err := uc.GetCompareResult(context.Background(), viewer, attempt)
	if err != nil {
		t.Fatalf("GetCompareResult: %v", err)
	}
	if res.Owner == nil || res.Owner.Login != "owner-login" {
		t.Errorf("Owner = %+v, want populated with owner-login for a different viewer", res.Owner)
	}
}

// --- UnsaveAttempt ---

func TestUnsaveAttempt_EmptyIDIsBadRequest(t *testing.T) {
	uc := NewComparisonUsecase(&stubRepo{}, &stubStore{}, nil, nil, nil)
	err := uc.UnsaveAttempt(context.Background(), uuid.NewV4(), "")
	if !errors.Is(err, comparison.ErrorBadRequest) {
		t.Errorf("err = %v, want ErrorBadRequest for empty attempt id", err)
	}
}

func TestUnsaveAttempt_DeletesVideoWhenPresent(t *testing.T) {
	user := uuid.NewV4()
	attempt := uuid.NewV4().String()
	repo := &stubRepo{hadVideo: true}
	store := &stubStore{}
	uc := NewComparisonUsecase(repo, store, nil, nil, nil)

	if err := uc.UnsaveAttempt(context.Background(), user, attempt); err != nil {
		t.Fatalf("UnsaveAttempt: %v", err)
	}
	if !repo.unsaveCalled {
		t.Error("repo.UnsaveAttempt was not called")
	}
	wantKey := "users/" + user.String() + "/" + attempt + "/video.mp4"
	if len(store.deleted) != 1 || store.deleted[0] != wantKey {
		t.Errorf("deleted = %v, want [%s]", store.deleted, wantKey)
	}
}

func TestUnsaveAttempt_NoVideoNoDelete(t *testing.T) {
	repo := &stubRepo{hadVideo: false}
	store := &stubStore{}
	uc := NewComparisonUsecase(repo, store, nil, nil, nil)

	if err := uc.UnsaveAttempt(context.Background(), uuid.NewV4(), uuid.NewV4().String()); err != nil {
		t.Fatalf("UnsaveAttempt: %v", err)
	}
	if len(store.deleted) != 0 {
		t.Errorf("deleted = %v, want no deletions when attempt had no video", store.deleted)
	}
}

func TestUnsaveAttempt_RepoErrorPropagates(t *testing.T) {
	repoErr := errors.New("db down")
	repo := &stubRepo{unsaveErr: repoErr}
	uc := NewComparisonUsecase(repo, &stubStore{}, nil, nil, nil)

	err := uc.UnsaveAttempt(context.Background(), uuid.NewV4(), uuid.NewV4().String())
	if !errors.Is(err, repoErr) {
		t.Errorf("err = %v, want the repo error to propagate", err)
	}
}

// --- GetLeaderboard ---

func TestGetLeaderboard_MarksMeWhenInTop(t *testing.T) {
	me := uuid.NewV4()
	other := uuid.NewV4()
	repo := &stubRepo{danceLB: []models.LeaderboardEntry{
		{UserID: other, Score: 90},
		{UserID: me, Score: 80},
	}}
	uc := NewComparisonUsecase(repo, &stubStore{}, nil, nil, nil)

	resp, err := uc.GetLeaderboard(context.Background(), "dance-1", &me)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if !resp.Top[1].IsMe {
		t.Error("entry for the requesting user was not flagged IsMe")
	}
	if resp.UserEntry != nil {
		t.Error("UserEntry should be nil when the user is already in the top")
	}
}

func TestGetLeaderboard_FetchesRankWhenNotInTop(t *testing.T) {
	me := uuid.NewV4()
	rank := &models.LeaderboardEntry{UserID: me, Rank: 42, Score: 30}
	repo := &stubRepo{
		danceLB:  []models.LeaderboardEntry{{UserID: uuid.NewV4(), Score: 90}},
		userRank: rank,
	}
	uc := NewComparisonUsecase(repo, &stubStore{}, nil, nil, nil)

	resp, err := uc.GetLeaderboard(context.Background(), "dance-1", &me)
	if err != nil {
		t.Fatalf("GetLeaderboard: %v", err)
	}
	if resp.UserEntry == nil || resp.UserEntry.Rank != 42 {
		t.Errorf("UserEntry = %+v, want rank 42 fetched separately", resp.UserEntry)
	}
}

func TestGetLeaderboard_RepoErrorPropagates(t *testing.T) {
	repo := &stubRepo{danceLBErr: errors.New("boom")}
	uc := NewComparisonUsecase(repo, &stubStore{}, nil, nil, nil)
	if _, err := uc.GetLeaderboard(context.Background(), "d", nil); err == nil {
		t.Error("expected error to propagate from GetDanceLeaderboard")
	}
}
