package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	platformrealtime "github.com/mugiew/onixggr/internal/platform/realtime"
)

func TestSendMessagePublishesGlobalEvent(t *testing.T) {
	repository := &stubRepository{}
	hub := &stubHub{}
	service := NewService(Options{
		Repository:      repository,
		Hub:             hub,
		Clock:           fixedClock{now: time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)},
		RetentionPeriod: 7 * 24 * time.Hour,
	})

	message, err := service.SendMessage(context.Background(), auth.Subject{
		UserID: "user-1",
		Role:   auth.RoleOwner,
	}, SendMessageInput{Body: "halo dunia"})
	if err != nil {
		t.Fatalf("SendMessage error = %v", err)
	}

	if message.Body != "halo dunia" {
		t.Fatalf("message.Body = %q, want halo dunia", message.Body)
	}
	if hub.lastEvent.Channel != globalChatChannel {
		t.Fatalf("channel = %q, want global_chat", hub.lastEvent.Channel)
	}
	if hub.lastEvent.Type != "chat.message.created" {
		t.Fatalf("type = %q, want chat.message.created", hub.lastEvent.Type)
	}
}

func TestDeleteMessageRequiresDevRole(t *testing.T) {
	service := NewService(Options{
		Repository: &stubRepository{},
		Clock:      fixedClock{now: time.Now()},
	})

	_, err := service.DeleteMessage(context.Background(), auth.Subject{
		UserID: "owner-1",
		Role:   auth.RoleOwner,
	}, "msg-1")
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("error = %v, want ErrForbidden", err)
	}
}

func TestDeleteMessagePublishesDeleteEvent(t *testing.T) {
	repository := &stubRepository{
		deleteMessage: Message{
			ID:             "msg-1",
			SenderUserID:   "user-1",
			SenderUsername: "owner-demo",
			SenderRole:     "owner",
			Body:           "hapus saya",
			DeletedAt:      timePtr(time.Date(2026, 3, 31, 10, 1, 0, 0, time.UTC)),
			CreatedAt:      time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC),
		},
	}
	hub := &stubHub{}
	service := NewService(Options{
		Repository: repository,
		Hub:        hub,
		Clock:      fixedClock{now: time.Date(2026, 3, 31, 10, 1, 0, 0, time.UTC)},
	})

	message, err := service.DeleteMessage(context.Background(), auth.Subject{
		UserID: "dev-1",
		Role:   auth.RoleDev,
	}, "msg-1")
	if err != nil {
		t.Fatalf("DeleteMessage error = %v", err)
	}

	if message.ID != "msg-1" {
		t.Fatalf("message.ID = %q, want msg-1", message.ID)
	}
	if hub.lastEvent.Type != "chat.message.deleted" {
		t.Fatalf("type = %q, want chat.message.deleted", hub.lastEvent.Type)
	}
}

func TestPruneExpiredUsesRetentionCutoff(t *testing.T) {
	repository := &stubRepository{}
	now := time.Date(2026, 3, 31, 10, 0, 0, 0, time.UTC)
	service := NewService(Options{
		Repository:      repository,
		Clock:           fixedClock{now: now},
		RetentionPeriod: 7 * 24 * time.Hour,
	})

	if _, err := service.PruneExpired(context.Background()); err != nil {
		t.Fatalf("PruneExpired error = %v", err)
	}

	want := now.Add(-7 * 24 * time.Hour)
	if !repository.lastPruneCutoff.Equal(want) {
		t.Fatalf("cutoff = %v, want %v", repository.lastPruneCutoff, want)
	}
}

type stubRepository struct {
	listMessages    []Message
	createMessage   Message
	deleteMessage   Message
	lastPruneCutoff time.Time
}

func (s *stubRepository) ListMessages(context.Context, time.Time, int) ([]Message, error) {
	return s.listMessages, nil
}

func (s *stubRepository) CreateMessage(_ context.Context, params CreateMessageParams) (Message, error) {
	if s.createMessage.ID == "" {
		s.createMessage = Message{
			ID:             "msg-1",
			SenderUserID:   params.SenderUserID,
			SenderUsername: "owner-demo",
			SenderRole:     "owner",
			Body:           params.Body,
			CreatedAt:      params.CreatedAt,
		}
	}

	return s.createMessage, nil
}

func (s *stubRepository) DeleteMessage(context.Context, DeleteMessageParams) (Message, error) {
	if s.deleteMessage.ID == "" {
		return Message{}, ErrNotFound
	}

	return s.deleteMessage, nil
}

func (s *stubRepository) PruneMessages(_ context.Context, cutoff time.Time) (int64, error) {
	s.lastPruneCutoff = cutoff
	return 2, nil
}

type stubHub struct {
	lastEvent platformrealtime.Event
}

func (s *stubHub) Publish(_ context.Context, event platformrealtime.Event) error {
	s.lastEvent = event
	return nil
}

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

func timePtr(value time.Time) *time.Time {
	return &value
}
