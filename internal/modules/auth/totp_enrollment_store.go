package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type EnrollmentStore interface {
	Save(ctx context.Context, userID string, secret string, ttl time.Duration) error
	Get(ctx context.Context, userID string) (string, error)
	Delete(ctx context.Context, userID string) error
}

type RedisEnrollmentStore struct {
	client *goredis.Client
}

func NewRedisEnrollmentStore(client *goredis.Client) *RedisEnrollmentStore {
	return &RedisEnrollmentStore{client: client}
}

func (s *RedisEnrollmentStore) Save(ctx context.Context, userID string, secret string, ttl time.Duration) error {
	if err := s.client.Set(ctx, enrollmentKey(userID), secret, ttl).Err(); err != nil {
		return fmt.Errorf("save totp enrollment secret: %w", err)
	}

	return nil
}

func (s *RedisEnrollmentStore) Get(ctx context.Context, userID string) (string, error) {
	secret, err := s.client.Get(ctx, enrollmentKey(userID)).Result()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return "", ErrNotFound
		}

		return "", fmt.Errorf("get totp enrollment secret: %w", err)
	}

	return secret, nil
}

func (s *RedisEnrollmentStore) Delete(ctx context.Context, userID string) error {
	if err := s.client.Del(ctx, enrollmentKey(userID)).Err(); err != nil {
		return fmt.Errorf("delete totp enrollment secret: %w", err)
	}

	return nil
}

func enrollmentKey(userID string) string {
	return "auth:totp:enrollment:" + userID
}
