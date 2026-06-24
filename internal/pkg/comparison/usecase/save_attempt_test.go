package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/comparison"
	"context"
	"encoding/json"
	"errors"
	"testing"

	uuid "github.com/satori/go.uuid"
)

// fakeCompRepo embeds the ComparisonRepo interface so only the methods that
// SaveAttempt actually exercises need real bodies. Any other call dispatches to
// the nil embedded interface and panics — that is intentional: it makes the
// test fail loudly if SaveAttempt grows an unexpected repo dependency.
type fakeCompRepo struct {
	comparison.ComparisonRepo
	prevRank *models.LeaderboardEntry

	saveCalled    bool
	savedScore    float64
	savedTiming   float64
	savedAmp      float64
	savedPose     float64
	recordedScore float64
}

func (f *fakeCompRepo) GetUserDanceRank(_ context.Context, _ string, _ uuid.UUID) (*models.LeaderboardEntry, error) {
	return f.prevRank, nil
}

func (f *fakeCompRepo) RecordDanceAttempt(_ context.Context, _ string, _ *uuid.UUID, _ string, score float64) error {
	f.recordedScore = score
	return nil
}

func (f *fakeCompRepo) SavedAttemptExists(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
	return false, nil
}

func (f *fakeCompRepo) SaveAttempt(_ context.Context, _ uuid.UUID, _, _ string, score float64, _ bool, _ string, _ bool, timingScore, amplitudeScore, poseScore float64) error {
	f.saveCalled = true
	f.savedScore = score
	f.savedTiming = timingScore
	f.savedAmp = amplitudeScore
	f.savedPose = poseScore
	return nil
}

// fakeStorage serves comparison_result.json blobs from an in-memory map and
// mimics the S3 "not found" error shape that isS3NotFoundError recognises.
type fakeStorage struct {
	comparison.ComparisonStorageRepo
	files map[string][]byte
}

func (f *fakeStorage) DownloadFile(_ context.Context, key string) ([]byte, error) {
	if d, ok := f.files[key]; ok {
		return d, nil
	}
	return nil, errors.New("NoSuchKey: the specified key does not exist")
}

func (f *fakeStorage) DeleteFile(_ context.Context, _ string) error { return nil }

func resultKey(userID, attemptID string) string {
	return "users/" + userID + "/" + attemptID + "/comparison_result.json"
}

// TestSaveAttempt_PersistsMLScoreNotClientScore is the regression test for the
// leaderboard-integrity fix: when the ML comparison_result.json exists, the
// persisted score comes from it, and a higher client-supplied score is ignored.
func TestSaveAttempt_PersistsMLScoreNotClientScore(t *testing.T) {
	userID := uuid.NewV4()
	attemptID := uuid.NewV4().String()

	mlResult := models.CompareStatusResult{
		ComparisonScore: 42.5,
		Segments: []models.SegmentDiagnostic{
			{TimingScore: 80, AmplitudeScore: 60, PoseAccuracyScore: 40},
			{TimingScore: 60, AmplitudeScore: 40, PoseAccuracyScore: 20},
		},
	}
	data, _ := json.Marshal(mlResult)
	storage := &fakeStorage{files: map[string][]byte{
		resultKey(userID.String(), attemptID): data,
	}}
	repo := &fakeCompRepo{}
	uc := NewComparisonUsecase(repo, storage, nil, nil, nil)

	cheatScore := 100.0
	err := uc.SaveAttempt(context.Background(), userID, attemptID, "dance-1", true,
		&cheatScore, "me", false, nil, false, false, 99, 99, 99)
	if err != nil {
		t.Fatalf("SaveAttempt returned error: %v", err)
	}

	if !repo.saveCalled {
		t.Fatal("expected SaveAttempt repo call")
	}
	if repo.savedScore != 42.5 {
		t.Errorf("persisted score = %v, want 42.5 (ML value, not client 100)", repo.savedScore)
	}
	if repo.recordedScore != 42.5 {
		t.Errorf("recorded attempt score = %v, want 42.5", repo.recordedScore)
	}
	// Segment means: timing (80+60)/2=70, amplitude (60+40)/2=50, pose (40+20)/2=30.
	if repo.savedTiming != 70 || repo.savedAmp != 50 || repo.savedPose != 30 {
		t.Errorf("segment metrics = (%v, %v, %v), want (70, 50, 30) derived from ML segments",
			repo.savedTiming, repo.savedAmp, repo.savedPose)
	}
}

// TestSaveAttempt_FallsBackToClientScoreWhenResultMissing covers the legacy /
// anon-handoff path: with no S3 artifact, the client-reported score is used and
// the client-reported segment metrics are preserved.
func TestSaveAttempt_FallsBackToClientScoreWhenResultMissing(t *testing.T) {
	userID := uuid.NewV4()
	attemptID := uuid.NewV4().String()

	storage := &fakeStorage{files: map[string][]byte{}}
	repo := &fakeCompRepo{}
	uc := NewComparisonUsecase(repo, storage, nil, nil, nil)

	clientScore := 77.0
	err := uc.SaveAttempt(context.Background(), userID, attemptID, "dance-1", true,
		&clientScore, "me", false, nil, false, false, 10, 20, 30)
	if err != nil {
		t.Fatalf("SaveAttempt returned error: %v", err)
	}

	if repo.savedScore != 77.0 {
		t.Errorf("persisted score = %v, want 77 (client fallback)", repo.savedScore)
	}
	if repo.savedTiming != 10 || repo.savedAmp != 20 || repo.savedPose != 30 {
		t.Errorf("segment metrics = (%v, %v, %v), want client-supplied (10, 20, 30) preserved",
			repo.savedTiming, repo.savedAmp, repo.savedPose)
	}
}

func TestMeanSegmentScores(t *testing.T) {
	t.Run("empty returns ok=false", func(t *testing.T) {
		if _, _, _, ok := meanSegmentScores(nil); ok {
			t.Error("ok = true for empty segments, want false")
		}
	})

	t.Run("averages across segments", func(t *testing.T) {
		timing, amp, pose, ok := meanSegmentScores([]models.SegmentDiagnostic{
			{TimingScore: 10, AmplitudeScore: 20, PoseAccuracyScore: 30},
			{TimingScore: 30, AmplitudeScore: 40, PoseAccuracyScore: 50},
		})
		if !ok {
			t.Fatal("ok = false, want true")
		}
		if timing != 20 || amp != 30 || pose != 40 {
			t.Errorf("means = (%v, %v, %v), want (20, 30, 40)", timing, amp, pose)
		}
	})
}
