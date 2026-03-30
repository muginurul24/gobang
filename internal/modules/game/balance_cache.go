package game

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type BalanceCache interface {
	Get(ctx context.Context, storeID string, memberID string) (GameBalanceResult, bool, error)
	Set(ctx context.Context, storeID string, memberID string, result GameBalanceResult, ttl time.Duration) error
}

type RedisBalanceCache struct {
	client *goredis.Client
}

func NewRedisBalanceCache(client *goredis.Client) *RedisBalanceCache {
	return &RedisBalanceCache{client: client}
}

func (c *RedisBalanceCache) Get(ctx context.Context, storeID string, memberID string) (GameBalanceResult, bool, error) {
	if c == nil || c.client == nil {
		return GameBalanceResult{}, false, nil
	}

	payload, err := c.client.Get(ctx, c.key(storeID, memberID)).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return GameBalanceResult{}, false, nil
		}

		return GameBalanceResult{}, false, fmt.Errorf("get game balance cache: %w", err)
	}

	var result GameBalanceResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return GameBalanceResult{}, false, fmt.Errorf("decode game balance cache: %w", err)
	}

	return result, true, nil
}

func (c *RedisBalanceCache) Set(ctx context.Context, storeID string, memberID string, result GameBalanceResult, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return nil
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode game balance cache: %w", err)
	}

	if err := c.client.Set(ctx, c.key(storeID, memberID), payload, ttl).Err(); err != nil {
		return fmt.Errorf("set game balance cache: %w", err)
	}

	return nil
}

func (c *RedisBalanceCache) key(storeID string, memberID string) string {
	return "game:balance:" + storeID + ":" + memberID
}

type noopBalanceCache struct{}

func (noopBalanceCache) Get(context.Context, string, string) (GameBalanceResult, bool, error) {
	return GameBalanceResult{}, false, nil
}

func (noopBalanceCache) Set(context.Context, string, string, GameBalanceResult, time.Duration) error {
	return nil
}
