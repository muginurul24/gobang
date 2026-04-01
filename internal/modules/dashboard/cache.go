package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type SummaryCache interface {
	Get(ctx context.Context, key string) (Summary, bool, error)
	Set(ctx context.Context, key string, summary Summary, ttl time.Duration) error
}

type RedisSummaryCache struct {
	client *goredis.Client
}

func NewRedisSummaryCache(client *goredis.Client) *RedisSummaryCache {
	return &RedisSummaryCache{client: client}
}

func (c *RedisSummaryCache) Get(ctx context.Context, key string) (Summary, bool, error) {
	if c == nil || c.client == nil {
		return Summary{}, false, nil
	}

	payload, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if err == goredis.Nil {
			return Summary{}, false, nil
		}

		return Summary{}, false, fmt.Errorf("get dashboard summary cache: %w", err)
	}

	var summary Summary
	if err := json.Unmarshal(payload, &summary); err != nil {
		return Summary{}, false, fmt.Errorf("decode dashboard summary cache: %w", err)
	}

	return summary, true, nil
}

func (c *RedisSummaryCache) Set(ctx context.Context, key string, summary Summary, ttl time.Duration) error {
	if c == nil || c.client == nil {
		return nil
	}

	payload, err := json.Marshal(summary)
	if err != nil {
		return fmt.Errorf("encode dashboard summary cache: %w", err)
	}

	if err := c.client.Set(ctx, key, payload, ttl).Err(); err != nil {
		return fmt.Errorf("set dashboard summary cache: %w", err)
	}

	return nil
}

type noopSummaryCache struct{}

func (noopSummaryCache) Get(context.Context, string) (Summary, bool, error) {
	return Summary{}, false, nil
}

func (noopSummaryCache) Set(context.Context, string, Summary, time.Duration) error {
	return nil
}
