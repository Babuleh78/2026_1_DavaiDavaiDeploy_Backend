package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"DDDance/internal/models"
	"DDDance/internal/pkg/kafka"
	"DDDance/internal/pkg/social"
	"DDDance/internal/pkg/users"

	uuid "github.com/satori/go.uuid"
)

type fakeSocialRepo struct {
	social.SocialRepo

	liked     bool
	toggleErr error
	count     int64
	countErr  error
	author    *models.DanceAuthor
	authorErr error
}

func (f *fakeSocialRepo) ToggleLike(_ context.Context, _ uuid.UUID, _ string) (bool, error) {
	return f.liked, f.toggleErr
}
func (f *fakeSocialRepo) GetLikesCount(_ context.Context, _ string) (int64, error) {
	return f.count, f.countErr
}
func (f *fakeSocialRepo) GetDanceAuthor(_ context.Context, _ string) (*models.DanceAuthor, error) {
	return f.author, f.authorErr
}

// chanAchTrigger signals on a channel when the (async) achievement check runs,
// so tests can deterministically assert it fired without racing the goroutine.
type chanAchTrigger struct {
	fired chan uuid.UUID
}

func (c *chanAchTrigger) CheckAndUnlockAchievements(_ context.Context, userID uuid.UUID) ([]models.Achievement, error) {
	c.fired <- userID
	return nil, nil
}

type recordingKafka struct {
	topic  string
	key    string
	called bool
}

func (k *recordingKafka) PublishAsync(_ context.Context, topic, key string, _ []byte, _ func(error)) {
	k.called = true
	k.topic = topic
	k.key = key
}

func TestToggleLike_EmptyDanceIDBadRequest(t *testing.T) {
	uc := NewSocialUsecase(&fakeSocialRepo{}, nil)
	_, err := uc.ToggleLike(context.Background(), uuid.NewV4(), "")
	if !errors.Is(err, users.ErrorBadRequest) {
		t.Errorf("err = %v, want ErrorBadRequest", err)
	}
}

func TestToggleLike_ResponseReflectsRepo(t *testing.T) {
	repo := &fakeSocialRepo{liked: true, count: 7}
	// nil achTrigger: the async path is a no-op, so no goroutine to wait on.
	uc := NewSocialUsecase(repo, nil)

	resp, err := uc.ToggleLike(context.Background(), uuid.NewV4(), "dance-1")
	if err != nil {
		t.Fatalf("ToggleLike: %v", err)
	}
	if !resp.Liked || resp.LikesCount != 7 || resp.DanceID != "dance-1" {
		t.Errorf("resp = %+v, want liked=true count=7 dance-1", resp)
	}
}

func TestToggleLike_TriggersAchievementForAuthorWhenLiked(t *testing.T) {
	authorID := uuid.NewV4()
	repo := &fakeSocialRepo{
		liked:  true,
		count:  1,
		author: &models.DanceAuthor{ID: authorID.String()},
	}
	trig := &chanAchTrigger{fired: make(chan uuid.UUID, 1)}
	uc := NewSocialUsecase(repo, trig)

	if _, err := uc.ToggleLike(context.Background(), uuid.NewV4(), "dance-1"); err != nil {
		t.Fatalf("ToggleLike: %v", err)
	}

	select {
	case got := <-trig.fired:
		if got != authorID {
			t.Errorf("achievement check fired for %v, want author %v", got, authorID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("achievement check did not fire for the dance author")
	}
}

func TestToggleLike_NoAchievementWhenUnliked(t *testing.T) {
	authorID := uuid.NewV4()
	repo := &fakeSocialRepo{
		liked:  false, // unlike → must NOT notify the author
		count:  0,
		author: &models.DanceAuthor{ID: authorID.String()},
	}
	trig := &chanAchTrigger{fired: make(chan uuid.UUID, 1)}
	uc := NewSocialUsecase(repo, trig)

	if _, err := uc.ToggleLike(context.Background(), uuid.NewV4(), "dance-1"); err != nil {
		t.Fatalf("ToggleLike: %v", err)
	}
	select {
	case <-trig.fired:
		t.Fatal("achievement check fired on unlike, want no trigger")
	case <-time.After(200 * time.Millisecond):
		// expected: nothing fired
	}
}

func TestToggleLike_PublishesToKafkaWhenConfigured(t *testing.T) {
	authorID := uuid.NewV4()
	repo := &fakeSocialRepo{liked: true, count: 1, author: &models.DanceAuthor{ID: authorID.String()}}
	trig := &chanAchTrigger{fired: make(chan uuid.UUID, 1)}
	kp := &recordingKafka{}
	uc := NewSocialUsecase(repo, trig)
	uc.SetKafkaProducer(kp)

	if _, err := uc.ToggleLike(context.Background(), uuid.NewV4(), "dance-1"); err != nil {
		t.Fatalf("ToggleLike: %v", err)
	}
	if !kp.called || kp.topic != kafka.TopicLikeToggled || kp.key != authorID.String() {
		t.Errorf("kafka publish = {called:%v topic:%q key:%q}, want topic %q key %q",
			kp.called, kp.topic, kp.key, kafka.TopicLikeToggled, authorID.String())
	}
	// With kafka configured, the direct achievement trigger must NOT run.
	select {
	case <-trig.fired:
		t.Error("direct achievement check fired even though kafka is configured")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestToggleLike_ToggleErrorPropagates(t *testing.T) {
	repo := &fakeSocialRepo{toggleErr: errors.New("db down")}
	uc := NewSocialUsecase(repo, nil)
	if _, err := uc.ToggleLike(context.Background(), uuid.NewV4(), "dance-1"); err == nil {
		t.Error("expected ToggleLike repo error to propagate")
	}
}

func TestToggleLike_CountErrorPropagates(t *testing.T) {
	repo := &fakeSocialRepo{liked: true, countErr: errors.New("count failed")}
	uc := NewSocialUsecase(repo, nil)
	if _, err := uc.ToggleLike(context.Background(), uuid.NewV4(), "dance-1"); err == nil {
		t.Error("expected GetLikesCount error to propagate")
	}
}
