package chat

import (
	"context"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/modules/auth"
	"github.com/mugiew/onixggr/internal/platform/clock"
	platformrealtime "github.com/mugiew/onixggr/internal/platform/realtime"
)

const (
	globalChatChannel = "global_chat"
	maxMessageLength  = 1000
)

type HubContract interface {
	Publish(ctx context.Context, event platformrealtime.Event) error
}

type Service interface {
	ListMessages(ctx context.Context, subject auth.Subject, limit int) ([]Message, error)
	SendMessage(ctx context.Context, subject auth.Subject, input SendMessageInput) (Message, error)
	DeleteMessage(ctx context.Context, subject auth.Subject, messageID string) (Message, error)
	PruneExpired(ctx context.Context) (int64, error)
}

type Options struct {
	Repository      RepositoryContract
	Hub             HubContract
	Clock           clock.Clock
	RetentionPeriod time.Duration
}

type service struct {
	repository      RepositoryContract
	hub             HubContract
	clock           clock.Clock
	retentionPeriod time.Duration
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	retention := options.RetentionPeriod
	if retention <= 0 {
		retention = 7 * 24 * time.Hour
	}

	return &service{
		repository:      options.Repository,
		hub:             options.Hub,
		clock:           now,
		retentionPeriod: retention,
	}
}

func (s *service) ListMessages(ctx context.Context, subject auth.Subject, limit int) ([]Message, error) {
	if !canAccessChat(subject) {
		return nil, ErrForbidden
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	return s.repository.ListMessages(ctx, s.cutoff(), limit)
}

func (s *service) SendMessage(ctx context.Context, subject auth.Subject, input SendMessageInput) (Message, error) {
	if !canAccessChat(subject) {
		return Message{}, ErrForbidden
	}

	body := strings.TrimSpace(input.Body)
	if body == "" || len(body) > maxMessageLength {
		return Message{}, ErrInvalidBody
	}

	message, err := s.repository.CreateMessage(ctx, CreateMessageParams{
		SenderUserID: subject.UserID,
		Body:         body,
		CreatedAt:    s.clock.Now().UTC(),
	})
	if err != nil {
		return Message{}, err
	}

	s.publish(ctx, "chat.message.created", map[string]any{
		"message": message,
	})

	return message, nil
}

func (s *service) DeleteMessage(ctx context.Context, subject auth.Subject, messageID string) (Message, error) {
	if subject.Role != auth.RoleDev {
		return Message{}, ErrForbidden
	}

	message, err := s.repository.DeleteMessage(ctx, DeleteMessageParams{
		MessageID:          messageID,
		DeletedByDevUserID: subject.UserID,
		DeletedAt:          s.clock.Now().UTC(),
	})
	if err != nil {
		return Message{}, err
	}

	s.publish(ctx, "chat.message.deleted", map[string]any{
		"message_id":        message.ID,
		"deleted_by_dev_id": subject.UserID,
		"deleted_at":        message.DeletedAt,
		"sender_username":   message.SenderUsername,
	})

	return message, nil
}

func (s *service) PruneExpired(ctx context.Context) (int64, error) {
	return s.repository.PruneMessages(ctx, s.cutoff())
}

func (s *service) publish(ctx context.Context, eventType string, payload map[string]any) {
	if s.hub == nil {
		return
	}

	_ = s.hub.Publish(ctx, platformrealtime.Event{
		Channel:   globalChatChannel,
		Type:      eventType,
		Payload:   payload,
		CreatedAt: s.clock.Now().UTC(),
	})
}

func (s *service) cutoff() time.Time {
	return s.clock.Now().UTC().Add(-s.retentionPeriod)
}

func canAccessChat(subject auth.Subject) bool {
	switch subject.Role {
	case auth.RoleOwner, auth.RoleKaryawan, auth.RoleDev, auth.RoleSuperadmin:
		return true
	default:
		return false
	}
}
