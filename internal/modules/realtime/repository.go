package realtime

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) ListAccessibleStoreIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id
		FROM stores
		WHERE owner_user_id = $1
			AND deleted_at IS NULL

		UNION

		SELECT ss.store_id
		FROM store_staff ss
		INNER JOIN stores s ON s.id = ss.store_id
		WHERE ss.user_id = $1
			AND s.deleted_at IS NULL

		ORDER BY 1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list accessible store ids: %w", err)
	}
	defer rows.Close()

	storeIDs := make([]string, 0)
	for rows.Next() {
		var storeID string
		if err := rows.Scan(&storeID); err != nil {
			return nil, fmt.Errorf("scan accessible store id: %w", err)
		}
		storeIDs = append(storeIDs, storeID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accessible store ids: %w", err)
	}

	return storeIDs, nil
}

func (r *Repository) ListAllActiveStoreIDs(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id
		FROM stores
		WHERE deleted_at IS NULL
		ORDER BY 1
	`)
	if err != nil {
		return nil, fmt.Errorf("list all active store ids: %w", err)
	}
	defer rows.Close()

	storeIDs := make([]string, 0)
	for rows.Next() {
		var storeID string
		if err := rows.Scan(&storeID); err != nil {
			return nil, fmt.Errorf("scan active store id: %w", err)
		}
		storeIDs = append(storeIDs, storeID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active store ids: %w", err)
	}

	return storeIDs, nil
}
