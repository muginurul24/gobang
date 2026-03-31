package main

import (
	"context"
	"fmt"

	"github.com/mugiew/onixggr/internal/platform/db"
)

func applyDevOnlySeed(ctx context.Context, pool *db.Pool) (int, error) {
	applied, err := db.ApplySQLDir(ctx, pool, "seeds/dev-only")
	if err != nil {
		return 0, fmt.Errorf("apply dev-only sql seeds: %w", err)
	}

	return applied, nil
}
