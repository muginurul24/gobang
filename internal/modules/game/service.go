package game

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
	"github.com/mugiew/onixggr/internal/platform/security"
)

type RepositoryContract interface {
	AuthenticateStore(ctx context.Context, tokenHash string) (StoreScope, error)
	FindStoreMemberByUsername(ctx context.Context, storeID string, username string) (StoreMember, error)
	HasUpstreamUserCode(ctx context.Context, upstreamUserCode string) (bool, error)
	CreateStoreMember(ctx context.Context, params CreateStoreMemberParams) (StoreMember, error)
	InsertAuditLog(ctx context.Context, storeID string, action string, targetID *string, payload map[string]any, ipAddress string, userAgent string, occurredAt time.Time) error
}

type UpstreamClient interface {
	UserCreate(ctx context.Context, input nexusggr.UserCreateInput) (nexusggr.UserCreateResult, error)
}

type Service interface {
	CreateUser(ctx context.Context, storeToken string, input CreateUserInput, metadata RequestMetadata) (StoreMember, error)
}

type service struct {
	repository  RepositoryContract
	upstream    UpstreamClient
	clock       clock.Clock
	codeFactory func() (string, error)
}

func NewService(repository RepositoryContract, upstream UpstreamClient, now clock.Clock) Service {
	if now == nil {
		now = clock.SystemClock{}
	}

	if upstream == nil {
		upstream = noopUpstream{}
	}

	return &service{
		repository:  repository,
		upstream:    upstream,
		clock:       now,
		codeFactory: newUpstreamUserCode,
	}
}

func (s *service) CreateUser(ctx context.Context, storeToken string, input CreateUserInput, metadata RequestMetadata) (StoreMember, error) {
	store, err := s.authenticateStore(ctx, storeToken)
	if err != nil {
		return StoreMember{}, err
	}

	username := normalizeUsername(input.Username)
	if !validUsername(username) {
		return StoreMember{}, ErrInvalidUsername
	}

	_, err = s.repository.FindStoreMemberByUsername(ctx, store.ID, username)
	switch {
	case err == nil:
		return StoreMember{}, ErrDuplicateUsername
	case errors.Is(err, ErrNotFound):
	default:
		return StoreMember{}, err
	}

	now := s.clock.Now().UTC()
	for range 8 {
		upstreamUserCode, err := s.codeFactory()
		if err != nil {
			return StoreMember{}, fmt.Errorf("generate upstream user code: %w", err)
		}

		exists, err := s.repository.HasUpstreamUserCode(ctx, upstreamUserCode)
		if err != nil {
			return StoreMember{}, err
		}
		if exists {
			continue
		}

		if _, err := s.upstream.UserCreate(ctx, nexusggr.UserCreateInput{
			UserCode: upstreamUserCode,
		}); err != nil {
			return StoreMember{}, err
		}

		member, err := s.repository.CreateStoreMember(ctx, CreateStoreMemberParams{
			StoreID:          store.ID,
			RealUsername:     username,
			UpstreamUserCode: upstreamUserCode,
			Status:           MemberStatusActive,
			OccurredAt:       now,
		})
		if err != nil {
			if errors.Is(err, ErrDuplicateUsername) {
				return StoreMember{}, err
			}

			if errors.Is(err, ErrDuplicateUpstreamUserCode) {
				return StoreMember{}, ErrCodeGenerationExhausted
			}

			return StoreMember{}, err
		}

		if err := s.repository.InsertAuditLog(ctx, store.ID, "game.user_created", &member.ID, map[string]any{
			"real_username":      member.RealUsername,
			"upstream_user_code": member.UpstreamUserCode,
			"origin":             "store_api",
		}, metadata.IPAddress, metadata.UserAgent, now); err != nil {
			return StoreMember{}, err
		}

		return member, nil
	}

	return StoreMember{}, ErrCodeGenerationExhausted
}

func (s *service) authenticateStore(ctx context.Context, storeToken string) (StoreScope, error) {
	token := strings.TrimSpace(storeToken)
	if token == "" {
		return StoreScope{}, ErrUnauthorized
	}

	store, err := s.repository.AuthenticateStore(ctx, security.HashStoreToken(token))
	if err != nil {
		return StoreScope{}, err
	}

	if store.DeletedAt != nil || store.Status != StoreStatusActive {
		return StoreScope{}, ErrStoreInactive
	}

	return store, nil
}

type noopUpstream struct{}

func (noopUpstream) UserCreate(context.Context, nexusggr.UserCreateInput) (nexusggr.UserCreateResult, error) {
	return nexusggr.UserCreateResult{}, nexusggr.ErrNotConfigured
}
