package exam

import (
	"sync"
	"time"
)

// rateLimiter es un limitador de ventana fija (1 minuto) por clave (IP).
type rateLimiter struct {
	mu          sync.Mutex
	perMinute   int
	windowStart map[string]time.Time
	counts      map[string]int
}

func newRateLimiter(perMinute int) *rateLimiter {
	return &rateLimiter{
		perMinute:   perMinute,
		windowStart: make(map[string]time.Time),
		counts:      make(map[string]int),
	}
}

func (l *rateLimiter) allow(key string) bool {
	if key == "" {
		return true
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	start, ok := l.windowStart[key]
	if !ok || now.Sub(start) >= time.Minute {
		l.windowStart[key] = now
		l.counts[key] = 1
		return true
	}

	if l.counts[key] >= l.perMinute {
		return false
	}

	l.counts[key]++
	return true
}
