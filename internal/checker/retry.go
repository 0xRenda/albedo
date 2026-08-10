package checker

import (
	"albedo-checker/internal/utils"
	"context"
	"time"
)

func WithRetry(ctx context.Context, fn func() error, maxRetries int, minDelay, maxDelay time.Duration) error {
	var err error
	for i := 0; i <= maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if i < maxRetries {
			delay := utils.CalculateBackoff(i, minDelay, maxDelay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return err
}
