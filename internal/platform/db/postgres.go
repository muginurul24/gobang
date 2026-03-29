package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mugiew/onixggr/internal/platform/config"
)

func Open(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	}

	if cfg.MaxIdleConns > 0 {
		minConns := cfg.MaxIdleConns
		if cfg.MaxOpenConns > 0 && minConns > cfg.MaxOpenConns {
			minConns = cfg.MaxOpenConns
		}

		poolConfig.MinConns = int32(minConns)
	}

	if cfg.ConnMaxLifetime > 0 {
		poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}
