package llm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"time"
)

type RetryConfig struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	Jitter      float64
	IsRetryable func(error) bool
	Sleep       func(time.Duration)
}

func Retry(ctx context.Context, cfg RetryConfig, fn func(context.Context) (Response, error)) (Response, error) {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = 100 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 2 * time.Second
	}
	if cfg.IsRetryable == nil {
		cfg.IsRetryable = DefaultIsRetryable
	}
	if cfg.Sleep == nil {
		cfg.Sleep = time.Sleep
	}
	var lastErr error
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < cfg.MaxAttempts; i++ {
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		resp, err := fn(ctx)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !cfg.IsRetryable(err) || i == cfg.MaxAttempts-1 {
			break
		}
		delay := backoffDelay(cfg.BaseDelay, cfg.MaxDelay, cfg.Jitter, i, r)
		select {
		case <-ctx.Done():
			return Response{}, ctx.Err()
		default:
			cfg.Sleep(delay)
		}
	}
	return Response{}, fmt.Errorf("llm retry failed: %w", lastErr)
}

func DefaultIsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var nerr net.Error
	if errors.As(err, &nerr) {
		return true
	}
	return true
}

func backoffDelay(base, max time.Duration, jitter float64, attempt int, r *rand.Rand) time.Duration {
	pow := math.Pow(2, float64(attempt))
	d := time.Duration(float64(base) * pow)
	if d > max {
		d = max
	}
	if jitter > 0 {
		j := time.Duration(float64(d) * jitter * r.Float64())
		return d + j
	}
	return d
}
