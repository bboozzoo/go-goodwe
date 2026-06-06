package et

import (
	"context"
	"time"
)

// backoff implements a simple exponential backoff.
func backoff(ctx context.Context, operation func() error) error {
	const (
		maxRetries  = 3
		initialWait = 500 * time.Millisecond
	)

	var lastErr error
	wait := initialWait

	for i := 0; i < maxRetries; i++ {
		err := operation()
		if err == nil {
			return nil
		}
		lastErr = err

		// If context is cancelled, exit immediately
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
			// Continue to next retry
		}

		wait *= 2
	}

	return lastErr
}
