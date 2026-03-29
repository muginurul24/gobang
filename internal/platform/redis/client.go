package redis

import (
	"context"
	"fmt"

	"github.com/mugiew/onixggr/internal/platform/config"
	goredis "github.com/redis/go-redis/v9"
)

func Open(ctx context.Context, cfg config.RedisConfig) (*goredis.Client, error) {
	options, err := goredis.ParseURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse redis config: %w", err)
	}

	if cfg.Password != "" {
		options.Password = cfg.Password
	}

	options.DB = cfg.DB

	client := goredis.NewClient(options)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	return client, nil
}
