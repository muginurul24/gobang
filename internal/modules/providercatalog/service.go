package providercatalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/mugiew/onixggr/internal/platform/clock"
	"github.com/mugiew/onixggr/internal/platform/nexusggr"
)

type RepositoryContract interface {
	ApplySnapshot(ctx context.Context, snapshot catalogSnapshot) (SyncSummary, error)
	ListProviders(ctx context.Context, filter ListProvidersFilter) ([]Provider, error)
	ListGames(ctx context.Context, filter ListGamesFilter) ([]Game, error)
}

type UpstreamClient interface {
	ProviderList(ctx context.Context) (nexusggr.ProviderListResult, error)
	GameList(ctx context.Context, providerCode string) (nexusggr.GameListResult, error)
}

type Service interface {
	Sync(ctx context.Context) (SyncSummary, error)
	ListProviders(ctx context.Context, filter ListProvidersFilter) ([]Provider, error)
	ListGames(ctx context.Context, filter ListGamesFilter) ([]Game, error)
}

type Options struct {
	Repository RepositoryContract
	Upstream   UpstreamClient
	Clock      clock.Clock
}

type service struct {
	repository RepositoryContract
	upstream   UpstreamClient
	clock      clock.Clock
}

func NewService(options Options) Service {
	now := options.Clock
	if now == nil {
		now = clock.SystemClock{}
	}

	upstream := options.Upstream
	if upstream == nil {
		upstream = noopUpstream{}
	}

	return &service{
		repository: options.Repository,
		upstream:   upstream,
		clock:      now,
	}
}

func (s *service) Sync(ctx context.Context) (SyncSummary, error) {
	providerList, err := s.upstream.ProviderList(ctx)
	if err != nil {
		return SyncSummary{}, err
	}

	syncedAt := s.clock.Now().UTC()
	snapshot := catalogSnapshot{
		SyncedAt:        syncedAt,
		Providers:       make([]snapshotProvider, 0, len(providerList.Providers)),
		GamesByProvider: make(map[string][]snapshotGame, len(providerList.Providers)),
	}

	for _, provider := range providerList.Providers {
		providerCode := normalizeProviderCode(provider.Code)
		if providerCode == "" {
			continue
		}

		snapshot.Providers = append(snapshot.Providers, snapshotProvider{
			ProviderCode: providerCode,
			ProviderName: strings.TrimSpace(provider.Name),
			Status:       provider.Status,
		})

		gameList, err := s.upstream.GameList(ctx, providerCode)
		if err != nil {
			return SyncSummary{}, fmt.Errorf("sync games for %s: %w", providerCode, err)
		}

		games := make([]snapshotGame, 0, len(gameList.Games))
		for _, game := range gameList.Games {
			gameCode := strings.TrimSpace(game.GameCode)
			if gameCode == "" {
				continue
			}

			games = append(games, snapshotGame{
				ProviderCode: providerCode,
				GameCode:     gameCode,
				GameName:     strings.TrimSpace(game.GameName),
				BannerURL:    strings.TrimSpace(game.Banner),
				Status:       game.Status,
			})
		}

		snapshot.GamesByProvider[providerCode] = games
	}

	return s.repository.ApplySnapshot(ctx, snapshot)
}

func (s *service) ListProviders(ctx context.Context, filter ListProvidersFilter) ([]Provider, error) {
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Limit = normalizeLimit(filter.Limit, 20, 100)
	return s.repository.ListProviders(ctx, filter)
}

func (s *service) ListGames(ctx context.Context, filter ListGamesFilter) ([]Game, error) {
	filter.ProviderCode = normalizeProviderCode(filter.ProviderCode)
	filter.Query = strings.TrimSpace(filter.Query)
	filter.Limit = normalizeLimit(filter.Limit, 50, 200)
	return s.repository.ListGames(ctx, filter)
}

func normalizeProviderCode(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeLimit(value int, fallback int, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}

	return value
}

type noopUpstream struct{}

func (noopUpstream) ProviderList(context.Context) (nexusggr.ProviderListResult, error) {
	return nexusggr.ProviderListResult{}, nexusggr.ErrNotConfigured
}

func (noopUpstream) GameList(context.Context, string) (nexusggr.GameListResult, error) {
	return nexusggr.GameListResult{}, nexusggr.ErrNotConfigured
}
