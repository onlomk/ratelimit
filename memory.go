package ratelimit

import (
	"context"
	"strconv"
	"sync"
	"time"
)

const (
	DefaultMemoryLimiterPrefix = "rate_limit"
	defaultMemoryLimiterTTL    = 5 * time.Minute
)

// MemoryLimiter is an in-process limiter with the same Rule and Algorithm
// contract as RedisLimiter. It is useful for single-process services, tests, or
// as a fallback when Redis is unavailable.
type MemoryLimiter struct {
	prefix string
	ttl    time.Duration
	now    func() time.Time

	mu     sync.Mutex
	states map[string]*memoryState
	stop   chan struct{}
}

type memoryState struct {
	lastSeen time.Time

	// Token bucket.
	tokens    float64
	timestamp time.Time

	// Fixed window and sliding window counter.
	windowID int64
	current  int
	previous int

	// Sliding window log.
	events []int64
}

// NewMemoryLimiter creates an in-memory limiter.
//
// ttl controls how long idle keys are retained. A non-positive ttl uses a safe
// default. Call Close when the limiter is no longer needed to stop cleanup.
func NewMemoryLimiter(prefix string, ttl time.Duration) *MemoryLimiter {
	if prefix == "" {
		prefix = DefaultMemoryLimiterPrefix
	}
	if ttl <= 0 {
		ttl = defaultMemoryLimiterTTL
	}
	l := &MemoryLimiter{
		prefix: prefix,
		ttl:    ttl,
		now:    time.Now,
		states: make(map[string]*memoryState),
		stop:   make(chan struct{}),
	}
	go l.cleanupLoop()
	return l
}

// Close stops the background cleanup goroutine.
func (l *MemoryLimiter) Close() {
	if l == nil {
		return
	}
	select {
	case <-l.stop:
		return
	default:
		close(l.stop)
	}
}

func (l *MemoryLimiter) Allow(_ context.Context, rule Rule) (bool, error) {
	if rule.Limit <= 0 || rule.Window <= 0 || rule.Key == "" {
		return true, nil
	}
	if l == nil {
		return true, nil
	}

	algorithm := rule.Algorithm
	if algorithm == "" {
		algorithm = TokenBucket
	}

	switch algorithm {
	case FixedWindow:
		return l.allowFixedWindow(rule), nil
	case SlidingWindow:
		return l.allowSlidingWindow(rule), nil
	case SlidingWindowCounter:
		return l.allowSlidingWindowCounter(rule), nil
	case TokenBucket:
		return l.allowTokenBucket(rule), nil
	default:
		return l.allowTokenBucket(rule), nil
	}
}

func (l *MemoryLimiter) allowFixedWindow(rule Rule) bool {
	windowMillis := durationMillis(rule.Window)
	now := l.now()
	windowID := now.UnixMilli() / windowMillis
	key := l.stateKey(FixedWindow, rule.Key, strconv.FormatInt(windowID, 10))

	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.getStateLocked(key, now)
	state.current++
	return state.current <= rule.Limit
}

func (l *MemoryLimiter) allowTokenBucket(rule Rule) bool {
	burst := rule.Burst
	if burst <= 0 {
		burst = rule.Limit
	}
	if burst <= 0 {
		return true
	}

	now := l.now()
	key := l.stateKey(TokenBucket, rule.Key, "")

	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.getStateLocked(key, now)
	if state.timestamp.IsZero() {
		state.tokens = float64(burst)
		state.timestamp = now
	}

	delta := now.Sub(state.timestamp)
	if delta < 0 {
		delta = 0
	}
	refillRate := float64(rule.Limit) / rule.Window.Seconds()
	state.tokens = minFloat(float64(burst), state.tokens+delta.Seconds()*refillRate)
	state.timestamp = now

	if state.tokens < 1 {
		return false
	}
	state.tokens--
	return true
}

func (l *MemoryLimiter) allowSlidingWindow(rule Rule) bool {
	windowMillis := durationMillis(rule.Window)
	now := l.now()
	nowMillis := now.UnixMilli()
	minScore := nowMillis - windowMillis
	key := l.stateKey(SlidingWindow, rule.Key, "")

	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.getStateLocked(key, now)
	keep := 0
	for _, event := range state.events {
		if event > minScore {
			state.events[keep] = event
			keep++
		}
	}
	state.events = state.events[:keep]
	if len(state.events) >= rule.Limit {
		return false
	}
	state.events = append(state.events, nowMillis)
	return true
}

func (l *MemoryLimiter) allowSlidingWindowCounter(rule Rule) bool {
	windowMillis := durationMillis(rule.Window)
	now := l.now()
	nowMillis := now.UnixMilli()
	windowID := nowMillis / windowMillis
	key := l.stateKey(SlidingWindowCounter, rule.Key, "")

	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.getStateLocked(key, now)
	if state.windowID == 0 {
		state.windowID = windowID
	}
	if windowID != state.windowID {
		if windowID == state.windowID+1 {
			state.previous = state.current
		} else {
			state.previous = 0
		}
		state.current = 0
		state.windowID = windowID
	}

	elapsed := nowMillis % windowMillis
	previousWeight := float64(windowMillis-elapsed) / float64(windowMillis)
	estimated := float64(state.current) + float64(state.previous)*previousWeight
	if estimated+1 > float64(rule.Limit) {
		return false
	}
	state.current++
	return true
}

func (l *MemoryLimiter) getStateLocked(key string, now time.Time) *memoryState {
	state, ok := l.states[key]
	if !ok {
		state = &memoryState{}
		l.states[key] = state
	}
	state.lastSeen = now
	return state
}

func (l *MemoryLimiter) stateKey(algorithm Algorithm, key, suffix string) string {
	if suffix == "" {
		return l.prefix + ":" + string(algorithm) + ":" + key
	}
	return l.prefix + ":" + string(algorithm) + ":" + key + ":" + suffix
}

func (l *MemoryLimiter) cleanupLoop() {
	ticker := time.NewTicker(l.ttl)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanup()
		case <-l.stop:
			return
		}
	}
}

func (l *MemoryLimiter) cleanup() {
	now := l.now()
	deadline := now.Add(-l.ttl)

	l.mu.Lock()
	defer l.mu.Unlock()

	for key, state := range l.states {
		if state.lastSeen.Before(deadline) {
			delete(l.states, key)
		}
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
