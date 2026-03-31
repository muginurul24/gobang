package realtime

import (
	"context"
	"fmt"
	"slices"

	"github.com/mugiew/onixggr/internal/modules/auth"
)

type RepositoryContract interface {
	ListAccessibleStoreIDs(ctx context.Context, userID string) ([]string, error)
	ListAllActiveStoreIDs(ctx context.Context) ([]string, error)
}

type Authenticator interface {
	AuthenticateAccessToken(ctx context.Context, rawToken string) (auth.Subject, error)
}

type Service interface {
	AuthorizeConnection(ctx context.Context, accessToken string) (ConnectionSession, error)
}

type Options struct {
	Repository       RepositoryContract
	Authenticator    Authenticator
	HeartbeatSeconds int
}

type service struct {
	repository       RepositoryContract
	authenticator    Authenticator
	heartbeatSeconds int
}

func NewService(options Options) Service {
	heartbeatSeconds := options.HeartbeatSeconds
	if heartbeatSeconds <= 0 {
		heartbeatSeconds = 30
	}

	return &service{
		repository:       options.Repository,
		authenticator:    options.Authenticator,
		heartbeatSeconds: heartbeatSeconds,
	}
}

func (s *service) AuthorizeConnection(ctx context.Context, accessToken string) (ConnectionSession, error) {
	subject, err := s.authenticator.AuthenticateAccessToken(ctx, accessToken)
	if err != nil {
		return ConnectionSession{}, err
	}

	channels := []string{
		userChannel(subject.UserID),
		globalChatChannel,
	}

	switch subject.Role {
	case auth.RoleOwner, auth.RoleKaryawan:
		storeIDs, err := s.repository.ListAccessibleStoreIDs(ctx, subject.UserID)
		if err != nil {
			return ConnectionSession{}, fmt.Errorf("list accessible store channels: %w", err)
		}

		for _, storeID := range storeIDs {
			channels = append(channels, storeChannel(storeID))
		}
	case auth.RoleDev:
		storeIDs, err := s.repository.ListAllActiveStoreIDs(ctx)
		if err != nil {
			return ConnectionSession{}, fmt.Errorf("list platform store channels for dev: %w", err)
		}
		for _, storeID := range storeIDs {
			channels = append(channels, storeChannel(storeID))
		}
		channels = append(channels, roleChannel("dev"))
	case auth.RoleSuperadmin:
		storeIDs, err := s.repository.ListAllActiveStoreIDs(ctx)
		if err != nil {
			return ConnectionSession{}, fmt.Errorf("list platform store channels for superadmin: %w", err)
		}
		for _, storeID := range storeIDs {
			channels = append(channels, storeChannel(storeID))
		}
		channels = append(channels, roleChannel("superadmin"))
	}

	channels = dedupeSorted(channels)

	return ConnectionSession{
		Subject:          subject,
		Channels:         channels,
		HeartbeatSeconds: s.heartbeatSeconds,
	}, nil
}

func userChannel(userID string) string {
	return "user:" + userID
}

func storeChannel(storeID string) string {
	return "store:" + storeID
}

func roleChannel(role string) string {
	return "role:" + role
}

func dedupeSorted(channels []string) []string {
	seen := make(map[string]struct{}, len(channels))
	normalized := make([]string, 0, len(channels))

	for _, channel := range channels {
		if channel == "" {
			continue
		}
		if _, exists := seen[channel]; exists {
			continue
		}

		seen[channel] = struct{}{}
		normalized = append(normalized, channel)
	}

	slices.Sort(normalized)
	return normalized
}
