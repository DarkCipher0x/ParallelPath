package api

import (
	"sync"
	"time"
)

// limiter allows 20 unauthenticated requests per address per minute (sign-in, enrolment), which is plenty for a
// real person and makes guessing invite codes or flooding challenges pointless.
type limiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
	n    int
	per  time.Duration
}

func newLimiter() *limiter { return &limiter{hits: map[string][]time.Time{}, n: 20, per: time.Minute} }

func (l *limiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.hits) > 10000 { // bound memory under attack
		l.hits = map[string][]time.Time{}
	}
	h := l.hits[key]
	cut := 0
	for cut < len(h) && now.Sub(h[cut]) > l.per {
		cut++
	}
	h = h[cut:]
	if len(h) >= l.n {
		l.hits[key] = h
		return false
	}
	l.hits[key] = append(h, now)
	return true
}
