package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type SessionStore interface {
	Save(ctx context.Context, state SessionState, ttl time.Duration) error
	Get(ctx context.Context, sessionJTI string) (SessionState, error)
	Delete(ctx context.Context, sessionJTI string) error
	DeleteMany(ctx context.Context, sessionJTIs []string) error
}

type RedisSessionStore struct {
	client *goredis.Client
}

func NewRedisSessionStore(client *goredis.Client) *RedisSessionStore {
	return &RedisSessionStore{client: client}
}

func (s *RedisSessionStore) Save(ctx context.Context, state SessionState, ttl time.Duration) error {
	payload, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal redis session state: %w", err)
	}

	if err := s.client.Set(ctx, sessionKey(state.SessionJTI), payload, ttl).Err(); err != nil {
		return fmt.Errorf("save redis session state: %w", err)
	}

	return nil
}

func (s *RedisSessionStore) Get(ctx context.Context, sessionJTI string) (SessionState, error) {
	payload, err := s.client.Get(ctx, sessionKey(sessionJTI)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return SessionState{}, ErrNotFound
		}

		return SessionState{}, fmt.Errorf("get redis session state: %w", err)
	}

	var state SessionState
	if err := json.Unmarshal(payload, &state); err != nil {
		return SessionState{}, fmt.Errorf("unmarshal redis session state: %w", err)
	}

	return state, nil
}

func (s *RedisSessionStore) Delete(ctx context.Context, sessionJTI string) error {
	if err := s.client.Del(ctx, sessionKey(sessionJTI)).Err(); err != nil {
		return fmt.Errorf("delete redis session state: %w", err)
	}

	return nil
}

func (s *RedisSessionStore) DeleteMany(ctx context.Context, sessionJTIs []string) error {
	if len(sessionJTIs) == 0 {
		return nil
	}

	keys := make([]string, 0, len(sessionJTIs))
	for _, sessionJTI := range sessionJTIs {
		keys = append(keys, sessionKey(sessionJTI))
	}

	if err := s.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("delete many redis session states: %w", err)
	}

	return nil
}

func sessionKey(sessionJTI string) string {
	return "auth:session:" + sessionJTI
}
