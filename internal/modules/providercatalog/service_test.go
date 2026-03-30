package providercatalog

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mugiew/onixggr/internal/platform/nexusggr"
)

func TestSyncBuildsSnapshotFromUpstream(t *testing.T) {
	repository := &fakeRepository{}
	upstream := &fakeUpstream{
		providers: nexusggr.ProviderListResult{
			Message: "SUCCESS",
			Providers: []nexusggr.Provider{
				{Code: "PRAGMATIC", Name: "Pragmatic Play", Status: 1},
				{Code: "CQ9", Name: "CQ9", Status: 0},
			},
		},
		gamesByProvider: map[string]nexusggr.GameListResult{
			"PRAGMATIC": {
				Message: "SUCCESS",
				Games: []nexusggr.Game{
					{GameCode: "vs20doghouse", GameName: "The Dog House", Banner: "https://img.test/doghouse.png", Status: 1},
				},
			},
			"CQ9": {
				Message: "SUCCESS",
				Games: []nexusggr.Game{
					{GameCode: "cq9-demo", GameName: "CQ9 Demo", Banner: "", Status: 0},
				},
			},
		},
	}

	service := NewService(Options{
		Repository: repository,
		Upstream:   upstream,
		Clock:      fixedClock{now: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC)},
	})

	summary, err := service.Sync(context.Background())
	if err != nil {
		t.Fatalf("Sync returned error: %v", err)
	}

	if summary.ProvidersSynced != 2 {
		t.Fatalf("ProvidersSynced = %d, want 2", summary.ProvidersSynced)
	}

	if summary.GamesSynced != 2 {
		t.Fatalf("GamesSynced = %d, want 2", summary.GamesSynced)
	}

	if len(repository.snapshot.Providers) != 2 {
		t.Fatalf("provider snapshot size = %d, want 2", len(repository.snapshot.Providers))
	}

	if len(repository.snapshot.GamesByProvider["PRAGMATIC"]) != 1 {
		t.Fatalf("PRAGMATIC games = %d, want 1", len(repository.snapshot.GamesByProvider["PRAGMATIC"]))
	}
}

func TestSyncReturnsProviderScopedGameListError(t *testing.T) {
	service := NewService(Options{
		Repository: &fakeRepository{},
		Upstream: &fakeUpstream{
			providers: nexusggr.ProviderListResult{
				Message: "SUCCESS",
				Providers: []nexusggr.Provider{
					{Code: "PRAGMATIC", Name: "Pragmatic Play", Status: 1},
				},
			},
			gameErrByProvider: map[string]error{
				"PRAGMATIC": errors.New("upstream exploded"),
			},
		},
	})

	_, err := service.Sync(context.Background())
	if err == nil || err.Error() != "sync games for PRAGMATIC: upstream exploded" {
		t.Fatalf("Sync error = %v, want provider-scoped error", err)
	}
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type fakeRepository struct {
	snapshot catalogSnapshot
}

func (r *fakeRepository) ApplySnapshot(_ context.Context, snapshot catalogSnapshot) (SyncSummary, error) {
	r.snapshot = snapshot

	games := 0
	for _, entries := range snapshot.GamesByProvider {
		games += len(entries)
	}

	return SyncSummary{
		ProvidersSynced: len(snapshot.Providers),
		GamesSynced:     games,
	}, nil
}

func (r *fakeRepository) ListProviders(_ context.Context, _ ListProvidersFilter) ([]Provider, error) {
	return nil, nil
}

func (r *fakeRepository) ListGames(_ context.Context, _ ListGamesFilter) ([]Game, error) {
	return nil, nil
}

type fakeUpstream struct {
	providers         nexusggr.ProviderListResult
	providerErr       error
	gamesByProvider   map[string]nexusggr.GameListResult
	gameErrByProvider map[string]error
}

func (u *fakeUpstream) ProviderList(_ context.Context) (nexusggr.ProviderListResult, error) {
	if u.providerErr != nil {
		return nexusggr.ProviderListResult{}, u.providerErr
	}

	return u.providers, nil
}

func (u *fakeUpstream) GameList(_ context.Context, providerCode string) (nexusggr.GameListResult, error) {
	if err, ok := u.gameErrByProvider[providerCode]; ok {
		return nexusggr.GameListResult{}, err
	}

	return u.gamesByProvider[providerCode], nil
}
