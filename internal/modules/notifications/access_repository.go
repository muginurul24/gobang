package notifications

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type AccessRepository struct {
	pool *pgxpool.Pool
}

func NewAccessRepository(pool *pgxpool.Pool) *AccessRepository {
	return &AccessRepository{pool: pool}
}

func (r *AccessRepository) HasStoreAccess(ctx context.Context, userID string, storeID string) (bool, error) {
	var allowed bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM stores s
			WHERE s.id = $2::uuid
				AND s.deleted_at IS NULL
				AND (
					s.owner_user_id = $1::uuid
					OR EXISTS (
						SELECT 1
						FROM store_staff ss
						WHERE ss.store_id = s.id
							AND ss.user_id = $1::uuid
					)
				)
		)
	`, strings.TrimSpace(userID), strings.TrimSpace(storeID)).Scan(&allowed)
	if err != nil {
		return false, fmt.Errorf("check notification store access: %w", err)
	}

	return allowed, nil
}
