package utils

import (
	"math/rand"
	"time"
)

func JitteredDelay(minMs, maxMs int) time.Duration {
	delay := minMs + rand.Intn(maxMs-minMs+1)
	return time.Duration(delay) * time.Millisecond
}

func CalculateBackoff(attempt int, minDelay, maxDelay time.Duration) time.Duration {
	base := minDelay * time.Duration(1<<attempt)
	if base > maxDelay {
		base = maxDelay
	}
	jitter := time.Duration(rand.Int63n(int64(base / 2)))
	delay := base + jitter
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}
