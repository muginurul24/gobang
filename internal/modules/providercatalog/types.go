package providercatalog

import "time"

type Provider struct {
	ProviderCode string    `json:"provider_code"`
	ProviderName string    `json:"provider_name"`
	Status       int       `json:"status"`
	SyncedAt     time.Time `json:"synced_at"`
}

type Game struct {
	ProviderCode string    `json:"provider_code"`
	GameCode     string    `json:"game_code"`
	GameName     string    `json:"game_name"`
	BannerURL    string    `json:"banner_url"`
	Status       int       `json:"status"`
	SyncedAt     time.Time `json:"synced_at"`
}

type SyncSummary struct {
	ProvidersSynced int `json:"providers_synced"`
	GamesSynced     int `json:"games_synced"`
}

type ListProvidersFilter struct {
	Query  string
	Status *int
	Limit  int
}

type ListGamesFilter struct {
	ProviderCode string
	Query        string
	Status       *int
	Limit        int
}

type snapshotProvider struct {
	ProviderCode string
	ProviderName string
	Status       int
}

type snapshotGame struct {
	ProviderCode string
	GameCode     string
	GameName     string
	BannerURL    string
	Status       int
}

type catalogSnapshot struct {
	SyncedAt        time.Time
	Providers       []snapshotProvider
	GamesByProvider map[string][]snapshotGame
}
