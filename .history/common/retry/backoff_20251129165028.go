package retry

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// Config 재시도 설정
type Config struct {
	MaxAttempts        int
	InitialInterval    time.Duration
	MaxInterval        time.Duration
	BackoffCoefficient float64
	MaxElapsedTime     time.Duration
}

// DefaultConfig 기본 재시도 설정
func DefaultConfig() Config {
	return Config{
		MaxAttempts:        5,
		InitialInterval:    time.Second,
		MaxInterval:        time.Minute,
		BackoffCoefficient: 2.0,
		MaxElapsedTime:     time.Minute * 5,
	}
}

// Do 재시도 실행
func Do(ctx context.Context, config Config, logger *zap.Logger, fn func() error) error {
	var lastErr error
	interval := config.InitialInterval
	startTime := time.Now()

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// 컨텍스트 취소 확인
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// 최대 경과 시간 확인
		if time.Since(startTime) > config.MaxElapsedTime {
			return fmt.Errorf("max elapsed time exceeded: %w", lastErr)
		}

		// 함수 실행
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
		logger.Warn("retry attempt failed",
			zap.Int("attempt", attempt),
			zap.Int("maxAttempts", config.MaxAttempts),
			zap.Error(err))

		// 마지막 시도이면 재시도 안함
		if attempt == config.MaxAttempts {
			break
		}

		// 백오프 대기
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}

		// 다음 인터벌 계산 (exponential backoff)
		interval = time.Duration(float64(interval) * config.BackoffCoefficient)
		if interval > config.MaxInterval {
			interval = config.MaxInterval
		}
	}

	return fmt.Errorf("max attempts reached: %w", lastErr)
}

// DoWithResult 재시도 실행 (결과 반환)
func DoWithResult[T any](ctx context.Context, config Config, logger *zap.Logger, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error
	interval := config.InitialInterval
	startTime := time.Now()

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}

		if time.Since(startTime) > config.MaxElapsedTime {
			return result, fmt.Errorf("max elapsed time exceeded: %w", lastErr)
		}

		result, err := fn()
		if err == nil {
			return result, nil
		}

		lastErr = err
		logger.Warn("retry attempt failed",
			zap.Int("attempt", attempt),
			zap.Int("maxAttempts", config.MaxAttempts),
			zap.Error(err))

		if attempt == config.MaxAttempts {
			break
		}

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(interval):
		}

		interval = time.Duration(float64(interval) * config.BackoffCoefficient)
		if interval > config.MaxInterval {
			interval = config.MaxInterval
		}
	}

	return result, fmt.Errorf("max attempts reached: %w", lastErr)
}

