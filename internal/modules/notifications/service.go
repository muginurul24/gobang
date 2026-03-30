package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	platformrealtime "github.com/mugiew/onixggr/internal/platform/realtime"
)

type HubContract interface {
	Publish(ctx context.Context, event platformrealtime.Event) error
}

type Service interface {
	Create(ctx context.Context, params CreateParams) (Notification, error)
	ListByScope(ctx context.Context, params ListParams) ([]Notification, error)
	MarkRead(ctx context.Context, id string) error
	CountUnread(ctx context.Context, scopeType ScopeType, scopeID string) (int, error)
}

type Options struct {
	Repository RepositoryContract
	Hub        HubContract
	Logger     *slog.Logger
}

type service struct {
	repository RepositoryContract
	hub        HubContract
	logger     *slog.Logger
}

func NewService(options Options) Service {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &service{
		repository: options.Repository,
		hub:        options.Hub,
		logger:     logger,
	}
}

func (s *service) Create(ctx context.Context, params CreateParams) (Notification, error) {
	if strings.TrimSpace(string(params.ScopeType)) == "" {
		return Notification{}, fmt.Errorf("notification scope_type is required")
	}
	if strings.TrimSpace(params.ScopeID) == "" {
		return Notification{}, fmt.Errorf("notification scope_id is required")
	}
	if strings.TrimSpace(params.EventType) == "" {
		return Notification{}, fmt.Errorf("notification event_type is required")
	}

	now := time.Now().UTC()
	notification, err := s.repository.Create(ctx, params, now)
	if err != nil {
		return Notification{}, err
	}

	s.pushToRealtime(ctx, notification)
	return notification, nil
}

func (s *service) ListByScope(ctx context.Context, params ListParams) ([]Notification, error) {
	return s.repository.ListByScope(ctx, params)
}

func (s *service) MarkRead(ctx context.Context, id string) error {
	return s.repository.MarkRead(ctx, id, time.Now().UTC())
}

func (s *service) CountUnread(ctx context.Context, scopeType ScopeType, scopeID string) (int, error) {
	return s.repository.CountUnread(ctx, scopeType, scopeID)
}

func (s *service) pushToRealtime(ctx context.Context, n Notification) {
	if s.hub == nil {
		return
	}

	channel := resolveChannel(n.ScopeType, n.ScopeID)
	if channel == "" {
		return
	}

	err := s.hub.Publish(ctx, platformrealtime.Event{
		Channel: channel,
		Type:    "notification." + n.EventType,
		Payload: map[string]any{
			"id":         n.ID,
			"scope_type": string(n.ScopeType),
			"scope_id":   n.ScopeID,
			"event_type": n.EventType,
			"title":      n.Title,
			"body":       n.Body,
			"created_at": n.CreatedAt.Format(time.RFC3339),
		},
		CreatedAt: n.CreatedAt,
	})
	if err != nil {
		s.logger.Warn("failed to push notification to realtime",
			"notification_id", n.ID,
			"channel", channel,
			"error", err,
		)
	}
}

func resolveChannel(scopeType ScopeType, scopeID string) string {
	switch scopeType {
	case ScopeStore:
		return "store:" + scopeID
	case ScopeUser:
		return "user:" + scopeID
	case ScopeRole:
		return "role:" + scopeID
	case ScopeGlobal:
		return "global_chat"
	default:
		return ""
	}
}
