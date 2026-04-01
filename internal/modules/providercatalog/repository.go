package providercatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ApplySnapshot(ctx context.Context, snapshot catalogSnapshot) (SyncSummary, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SyncSummary{}, fmt.Errorf("begin provider catalog sync tx: %w", err)
	}
	defer tx.Rollback(ctx)

	providerCodes := make([]string, 0, len(snapshot.Providers))
	totalGames := 0

	for _, provider := range snapshot.Providers {
		providerCodes = append(providerCodes, provider.ProviderCode)
		if _, err := tx.Exec(ctx, `
			INSERT INTO provider_catalogs (
				provider_code,
				provider_name,
				status,
				synced_at,
				created_at,
				updated_at
			)
			VALUES ($1, $2, $3, $4, $4, $4)
			ON CONFLICT (provider_code) DO UPDATE
			SET
				provider_name = EXCLUDED.provider_name,
				status = EXCLUDED.status,
				synced_at = EXCLUDED.synced_at,
				updated_at = EXCLUDED.updated_at
		`, provider.ProviderCode, provider.ProviderName, provider.Status, snapshot.SyncedAt); err != nil {
			return SyncSummary{}, fmt.Errorf("upsert provider %s: %w", provider.ProviderCode, err)
		}

		games := snapshot.GamesByProvider[provider.ProviderCode]
		gameCodes := make([]string, 0, len(games))
		for _, game := range games {
			gameCodes = append(gameCodes, game.GameCode)
			totalGames++

			if _, err := tx.Exec(ctx, `
				INSERT INTO provider_games (
					provider_code,
					game_code,
					game_name,
					banner_url,
					status,
					synced_at,
					created_at,
					updated_at
				)
				VALUES ($1, $2, $3::jsonb, $4, $5, $6, $6, $6)
				ON CONFLICT (provider_code, game_code) DO UPDATE
				SET
					game_name = EXCLUDED.game_name,
					banner_url = EXCLUDED.banner_url,
					status = EXCLUDED.status,
					synced_at = EXCLUDED.synced_at,
					updated_at = EXCLUDED.updated_at
			`, game.ProviderCode, game.GameCode, toGameNameJSON(game.GameName), nullableString(game.BannerURL), game.Status, snapshot.SyncedAt); err != nil {
				return SyncSummary{}, fmt.Errorf("upsert game %s/%s: %w", game.ProviderCode, game.GameCode, err)
			}
		}

		if _, err := tx.Exec(ctx, `
			UPDATE provider_games
			SET
				status = 0,
				synced_at = $3,
				updated_at = $3
			WHERE provider_code = $1
				AND (cardinality($2::text[]) = 0 OR game_code <> ALL($2::text[]))
		`, provider.ProviderCode, gameCodes, snapshot.SyncedAt); err != nil {
			return SyncSummary{}, fmt.Errorf("mark missing games for %s: %w", provider.ProviderCode, err)
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE provider_catalogs
		SET
			status = 0,
			synced_at = $2,
			updated_at = $2
		WHERE cardinality($1::text[]) = 0 OR provider_code <> ALL($1::text[])
	`, providerCodes, snapshot.SyncedAt); err != nil {
		return SyncSummary{}, fmt.Errorf("mark missing providers: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE provider_games
		SET
			status = 0,
			synced_at = $2,
			updated_at = $2
		WHERE cardinality($1::text[]) = 0 OR provider_code <> ALL($1::text[])
	`, providerCodes, snapshot.SyncedAt); err != nil {
		return SyncSummary{}, fmt.Errorf("mark missing provider games: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return SyncSummary{}, fmt.Errorf("commit provider catalog sync tx: %w", err)
	}

	return SyncSummary{
		ProvidersSynced: len(snapshot.Providers),
		GamesSynced:     totalGames,
	}, nil
}

func (r *Repository) ListProviders(ctx context.Context, filter ListProvidersFilter) (ProviderPage, error) {
	search := "%" + filter.Query + "%"
	var totalCount int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int
		FROM provider_catalogs
		WHERE ($1 = '' OR provider_code ILIKE $2 OR provider_name ILIKE $2)
			AND ($3::int IS NULL OR status = $3)
	`, filter.Query, search, filter.Status).Scan(&totalCount); err != nil {
		return ProviderPage{}, fmt.Errorf("count provider catalog: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			provider_code,
			provider_name,
			status,
			synced_at
		FROM provider_catalogs
		WHERE ($1 = '' OR provider_code ILIKE $2 OR provider_name ILIKE $2)
			AND ($3::int IS NULL OR status = $3)
		ORDER BY provider_code ASC
		LIMIT $4
		OFFSET $5
	`, filter.Query, search, filter.Status, filter.Limit, filter.Offset)
	if err != nil {
		return ProviderPage{}, fmt.Errorf("list provider catalog: %w", err)
	}
	defer rows.Close()

	providers := []Provider{}
	for rows.Next() {
		var provider Provider
		if err := rows.Scan(
			&provider.ProviderCode,
			&provider.ProviderName,
			&provider.Status,
			&provider.SyncedAt,
		); err != nil {
			return ProviderPage{}, fmt.Errorf("scan provider catalog: %w", err)
		}

		providers = append(providers, provider)
	}

	if err := rows.Err(); err != nil {
		return ProviderPage{}, fmt.Errorf("iterate provider catalog: %w", err)
	}

	return ProviderPage{
		Items:      providers,
		TotalCount: totalCount,
		Limit:      filter.Limit,
		Offset:     filter.Offset,
	}, nil
}

func (r *Repository) ListGames(ctx context.Context, filter ListGamesFilter) (GamePage, error) {
	search := "%" + filter.Query + "%"
	var totalCount int
	if err := r.pool.QueryRow(ctx, `
		SELECT count(*)::int
		FROM provider_games
		WHERE ($1 = '' OR provider_code = $1)
			AND ($2 = '' OR game_code ILIKE $3 OR COALESCE(game_name->>'default', '') ILIKE $3)
			AND ($4::int IS NULL OR status = $4)
	`, filter.ProviderCode, filter.Query, search, filter.Status).Scan(&totalCount); err != nil {
		return GamePage{}, fmt.Errorf("count provider games: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			provider_code,
			game_code,
			COALESCE(game_name->>'default', ''),
			COALESCE(banner_url, ''),
			status,
			synced_at
		FROM provider_games
		WHERE ($1 = '' OR provider_code = $1)
			AND ($2 = '' OR game_code ILIKE $3 OR COALESCE(game_name->>'default', '') ILIKE $3)
			AND ($4::int IS NULL OR status = $4)
		ORDER BY provider_code ASC, game_code ASC
		LIMIT $5
		OFFSET $6
	`, filter.ProviderCode, filter.Query, search, filter.Status, filter.Limit, filter.Offset)
	if err != nil {
		return GamePage{}, fmt.Errorf("list provider games: %w", err)
	}
	defer rows.Close()

	games := []Game{}
	for rows.Next() {
		var game Game
		if err := rows.Scan(
			&game.ProviderCode,
			&game.GameCode,
			&game.GameName,
			&game.BannerURL,
			&game.Status,
			&game.SyncedAt,
		); err != nil {
			return GamePage{}, fmt.Errorf("scan provider game: %w", err)
		}

		games = append(games, game)
	}

	if err := rows.Err(); err != nil {
		return GamePage{}, fmt.Errorf("iterate provider games: %w", err)
	}

	return GamePage{
		Items:      games,
		TotalCount: totalCount,
		Limit:      filter.Limit,
		Offset:     filter.Offset,
	}, nil
}

func nullableString(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

func toGameNameJSON(value string) string {
	encoded, err := json.Marshal(map[string]string{
		"default": strings.TrimSpace(value),
	})
	if err != nil {
		return `{"default":""}`
	}

	return string(encoded)
}

func fromGameNameJSON(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}

	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err == nil {
		return strings.TrimSpace(payload["default"])
	}

	return strings.TrimSpace(string(raw))
}

func zeroTimeOrNow(value time.Time) time.Time {
	if value.IsZero() {
		return time.Now().UTC()
	}

	return value
}
