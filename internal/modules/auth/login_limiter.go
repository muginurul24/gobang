package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type LimitStatus struct {
	Limited    bool
	Scope      string
	RetryAfter time.Duration
}

type LoginLimiter interface {
	Check(ctx context.Context, ip string, identifier string) (LimitStatus, error)
	RegisterFailure(ctx context.Context, ip string, identifier string) error
	Reset(ctx context.Context, ip string, identifier string) error
}

type RedisLoginLimiter struct {
	client           *goredis.Client
	window           time.Duration
	maxAttemptsPerIP int
	maxAttemptsPerID int
}

func NewRedisLoginLimiter(client *goredis.Client, window time.Duration, maxAttemptsPerIP int, maxAttemptsPerID int) *RedisLoginLimiter {
	return &RedisLoginLimiter{
		client:           client,
		window:           window,
		maxAttemptsPerIP: maxAttemptsPerIP,
		maxAttemptsPerID: maxAttemptsPerID,
	}
}

func (l *RedisLoginLimiter) Check(ctx context.Context, ip string, identifier string) (LimitStatus, error) {
	normalizedIP := limiterIP(ip)
	normalizedIdentifier := normalizeLogin(identifier)

	ipCount, ipTTL, err := l.current(ctx, loginIPKey(normalizedIP))
	if err != nil {
		return LimitStatus{}, err
	}

	idCount, idTTL, err := l.current(ctx, loginIdentifierKey(normalizedIdentifier))
	if err != nil {
		return LimitStatus{}, err
	}

	if l.maxAttemptsPerID > 0 && idCount >= int64(l.maxAttemptsPerID) {
		return LimitStatus{
			Limited:    true,
			Scope:      "identifier",
			RetryAfter: normalizeRetryAfter(idTTL, l.window),
		}, nil
	}

	if l.maxAttemptsPerIP > 0 && ipCount >= int64(l.maxAttemptsPerIP) {
		return LimitStatus{
			Limited:    true,
			Scope:      "ip",
			RetryAfter: normalizeRetryAfter(ipTTL, l.window),
		}, nil
	}

	return LimitStatus{}, nil
}

func (l *RedisLoginLimiter) RegisterFailure(ctx context.Context, ip string, identifier string) error {
	normalizedIP := limiterIP(ip)
	normalizedIdentifier := normalizeLogin(identifier)

	if err := l.bump(ctx, loginIPKey(normalizedIP)); err != nil {
		return err
	}

	if err := l.bump(ctx, loginIdentifierKey(normalizedIdentifier)); err != nil {
		return err
	}

	return nil
}

func (l *RedisLoginLimiter) Reset(ctx context.Context, ip string, identifier string) error {
	normalizedIP := limiterIP(ip)
	normalizedIdentifier := normalizeLogin(identifier)

	if err := l.client.Del(ctx, loginIPKey(normalizedIP), loginIdentifierKey(normalizedIdentifier)).Err(); err != nil {
		return fmt.Errorf("reset login limiter: %w", err)
	}

	return nil
}

func (l *RedisLoginLimiter) current(ctx context.Context, key string) (int64, time.Duration, error) {
	count, err := l.client.Get(ctx, key).Int64()
	if err != nil && !errorsIsRedisNil(err) {
		return 0, 0, fmt.Errorf("get login limiter count: %w", err)
	}

	ttl, err := l.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, 0, fmt.Errorf("get login limiter ttl: %w", err)
	}

	return count, ttl, nil
}

func (l *RedisLoginLimiter) bump(ctx context.Context, key string) error {
	count, err := l.client.Incr(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("increment login limiter: %w", err)
	}

	if count == 1 {
		if err := l.client.Expire(ctx, key, l.window).Err(); err != nil {
			return fmt.Errorf("set login limiter expiry: %w", err)
		}
	}

	return nil
}

func loginIPKey(ip string) string {
	return "auth:login:limit:ip:" + ip
}

func loginIdentifierKey(identifier string) string {
	return "auth:login:limit:identifier:" + identifier
}

func limiterIP(ip string) string {
	trimmed := strings.TrimSpace(ip)
	if trimmed == "" {
		return "0.0.0.0"
	}

	return trimmed
}

func normalizeRetryAfter(ttl time.Duration, fallback time.Duration) time.Duration {
	if ttl <= 0 {
		return fallback
	}

	return ttl
}

func errorsIsRedisNil(err error) bool {
	return err == goredis.Nil
}
