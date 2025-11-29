package idempotency

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Store 멱등성 키 저장소 인터페이스
type Store interface {
	// Reserve 멱등성 키를 예약 (이미 존재하면 false 반환)
	Reserve(ctx context.Context, key string, ttl time.Duration) (bool, error)
	// IsProcessed 이미 처리된 키인지 확인
	IsProcessed(ctx context.Context, key string) (bool, error)
	// Release 멱등성 키 해제
	Release(ctx context.Context, key string) error
}

// RedisStore Redis 기반 멱등성 저장소
type RedisStore struct {
	client *redis.Client
	prefix string
}

// NewRedisStore Redis 기반 멱등성 저장소 생성
func NewRedisStore(client *redis.Client, prefix string) *RedisStore {
	return &RedisStore{
		client: client,
		prefix: prefix,
	}
}

// Reserve 멱등성 키 예약
func (s *RedisStore) Reserve(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	fullKey := s.getFullKey(key)
	result, err := s.client.SetNX(ctx, fullKey, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("failed to reserve idempotency key: %w", err)
	}
	return result, nil
}

// IsProcessed 이미 처리된 키인지 확인
func (s *RedisStore) IsProcessed(ctx context.Context, key string) (bool, error) {
	fullKey := s.getFullKey(key)
	exists, err := s.client.Exists(ctx, fullKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check idempotency key: %w", err)
	}
	return exists > 0, nil
}

// Release 멱등성 키 해제
func (s *RedisStore) Release(ctx context.Context, key string) error {
	fullKey := s.getFullKey(key)
	_, err := s.client.Del(ctx, fullKey).Result()
	if err != nil {
		return fmt.Errorf("failed to release idempotency key: %w", err)
	}
	return nil
}

func (s *RedisStore) getFullKey(key string) string {
	return fmt.Sprintf("%s:%s", s.prefix, key)
}

