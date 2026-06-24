package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"DDDance/internal/models"
	"DDDance/internal/pkg/notifications"

	uuid "github.com/satori/go.uuid"
)

type fakeNotifRepo struct {
	notifications.NotificationsRepo

	createErr   error
	createCalls int

	telegramID  *int64
	telegramErr error

	items   []models.Notification
	itemErr error
}

func (f *fakeNotifRepo) CreateDuelNotification(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _, _ string) error {
	f.createCalls++
	return f.createErr
}
func (f *fakeNotifRepo) GetUserTelegramID(_ context.Context, _ uuid.UUID) (*int64, error) {
	return f.telegramID, f.telegramErr
}
func (f *fakeNotifRepo) GetNotifications(_ context.Context, _ uuid.UUID) ([]models.Notification, error) {
	return f.items, f.itemErr
}

type fakeBot struct {
	pushed  [][]byte
	pushErr error
}

func (b *fakeBot) Push(_ context.Context, notification []byte) error {
	b.pushed = append(b.pushed, notification)
	return b.pushErr
}

func TestSend_NoBotNotifierStillStores(t *testing.T) {
	repo := &fakeNotifRepo{}
	uc := NewNotificationsUsecase(repo)

	if err := uc.Send(context.Background(), uuid.NewV4(), nil, "duel_invite", "duel-1", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if repo.createCalls != 1 {
		t.Errorf("CreateDuelNotification calls = %d, want 1", repo.createCalls)
	}
}

func TestSend_StoreErrorDoesNotFail(t *testing.T) {
	// A DB write failure must be swallowed — notifications are best-effort.
	repo := &fakeNotifRepo{createErr: errors.New("db down")}
	uc := NewNotificationsUsecase(repo)
	if err := uc.Send(context.Background(), uuid.NewV4(), nil, "duel_invite", "d", nil); err != nil {
		t.Errorf("Send returned %v, want nil despite store failure", err)
	}
}

func TestSend_NoTelegramIDSkipsPush(t *testing.T) {
	repo := &fakeNotifRepo{telegramID: nil}
	bot := &fakeBot{}
	uc := NewNotificationsUsecase(repo)
	uc.SetBotNotifier(bot)

	if err := uc.Send(context.Background(), uuid.NewV4(), nil, "duel_invite", "d", nil); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(bot.pushed) != 0 {
		t.Errorf("bot push count = %d, want 0 when user has no telegram id", len(bot.pushed))
	}
}

func TestSend_PushesWhenTelegramLinked(t *testing.T) {
	tid := int64(555)
	repo := &fakeNotifRepo{telegramID: &tid}
	bot := &fakeBot{}
	uc := NewNotificationsUsecase(repo)
	uc.SetBotNotifier(bot)

	err := uc.Send(context.Background(), uuid.NewV4(), nil, "duel_accept", "duel-9",
		map[string]any{"duel_id": "duel-9"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if len(bot.pushed) != 1 {
		t.Fatalf("bot push count = %d, want 1", len(bot.pushed))
	}

	var msg struct {
		TelegramID int64          `json:"telegram_id"`
		Type       string         `json:"type"`
		Payload    map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(bot.pushed[0], &msg); err != nil {
		t.Fatalf("pushed payload not valid JSON: %v", err)
	}
	if msg.TelegramID != 555 || msg.Type != "duel_accept" {
		t.Errorf("pushed msg = %+v, want telegram_id 555 / type duel_accept", msg)
	}
}

func TestSend_PushErrorSwallowed(t *testing.T) {
	tid := int64(1)
	repo := &fakeNotifRepo{telegramID: &tid}
	bot := &fakeBot{pushErr: errors.New("bot unreachable")}
	uc := NewNotificationsUsecase(repo)
	uc.SetBotNotifier(bot)

	if err := uc.Send(context.Background(), uuid.NewV4(), nil, "x", "d", nil); err != nil {
		t.Errorf("Send returned %v, want nil despite push failure", err)
	}
}

func TestGetUnread_CountsUnread(t *testing.T) {
	repo := &fakeNotifRepo{items: []models.Notification{
		{ID: 1, IsRead: false},
		{ID: 2, IsRead: true},
		{ID: 3, IsRead: false},
	}}
	uc := NewNotificationsUsecase(repo)

	items, unread, err := uc.GetUnread(context.Background(), uuid.NewV4())
	if err != nil {
		t.Fatalf("GetUnread: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("items = %d, want 3", len(items))
	}
	if unread != 2 {
		t.Errorf("unread = %d, want 2", unread)
	}
}

func TestGetUnread_RepoErrorPropagates(t *testing.T) {
	repo := &fakeNotifRepo{itemErr: errors.New("boom")}
	uc := NewNotificationsUsecase(repo)
	if _, _, err := uc.GetUnread(context.Background(), uuid.NewV4()); err == nil {
		t.Error("expected error to propagate from GetNotifications")
	}
}
