package proxy

import (
	"context"
	"time"
)

func (m *Manager) StartHealthChecker(ctx context.Context, checkURL string, interval, timeout time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.HealthCheck(ctx, checkURL, timeout)
		}
	}
}
