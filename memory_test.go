package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryLimiterTokenBucket(t *testing.T) {
	limiter := NewMemoryLimiter("test", time.Minute)
	defer limiter.Close()

	rule := Rule{Key: "client", Limit: 2, Burst: 2, Window: time.Second, Algorithm: TokenBucket}
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(context.Background(), rule)
		if err != nil {
			t.Fatalf("Allow returned error: %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	allowed, err := limiter.Allow(context.Background(), rule)
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if allowed {
		t.Fatal("third request should be rejected")
	}
}

func TestMemoryLimiterFixedWindow(t *testing.T) {
	limiter := NewMemoryLimiter("test", time.Minute)
	defer limiter.Close()

	rule := Rule{Key: "client", Limit: 1, Window: time.Minute, Algorithm: FixedWindow}
	allowed, err := limiter.Allow(context.Background(), rule)
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if !allowed {
		t.Fatal("first request should be allowed")
	}

	allowed, err = limiter.Allow(context.Background(), rule)
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if allowed {
		t.Fatal("second request should be rejected")
	}
}

func TestMemoryLimiterInvalidRuleIsUnlimited(t *testing.T) {
	limiter := NewMemoryLimiter("test", time.Minute)
	defer limiter.Close()

	allowed, err := limiter.Allow(context.Background(), Rule{Key: "client", Limit: 0, Window: time.Second})
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if !allowed {
		t.Fatal("invalid rule should be allowed")
	}
}

func TestMemoryLimiterSlidingWindow(t *testing.T) {
	limiter := NewMemoryLimiter("test", time.Minute)
	defer limiter.Close()

	now := time.Unix(100, 0)
	limiter.now = func() time.Time { return now }

	rule := Rule{Key: "client", Limit: 2, Window: time.Second, Algorithm: SlidingWindow}
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(context.Background(), rule)
		if err != nil {
			t.Fatalf("Allow returned error: %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	allowed, err := limiter.Allow(context.Background(), rule)
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if allowed {
		t.Fatal("third request in the same sliding window should be rejected")
	}

	now = now.Add(time.Second + time.Millisecond)
	allowed, err = limiter.Allow(context.Background(), rule)
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if !allowed {
		t.Fatal("request after old events expire should be allowed")
	}
}

func TestMemoryLimiterSlidingWindowCounter(t *testing.T) {
	limiter := NewMemoryLimiter("test", time.Minute)
	defer limiter.Close()

	now := time.Unix(100, 0)
	limiter.now = func() time.Time { return now }

	rule := Rule{Key: "client", Limit: 2, Window: time.Second, Algorithm: SlidingWindowCounter}
	for i := 0; i < 2; i++ {
		allowed, err := limiter.Allow(context.Background(), rule)
		if err != nil {
			t.Fatalf("Allow returned error: %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	allowed, err := limiter.Allow(context.Background(), rule)
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if allowed {
		t.Fatal("third request in the same counter window should be rejected")
	}

	now = time.Unix(100, 0).Add(1100 * time.Millisecond)
	allowed, err = limiter.Allow(context.Background(), rule)
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if allowed {
		t.Fatal("request should still be rejected while previous window has high weight")
	}

	now = time.Unix(100, 0).Add(1600 * time.Millisecond)
	allowed, err = limiter.Allow(context.Background(), rule)
	if err != nil {
		t.Fatalf("Allow returned error: %v", err)
	}
	if !allowed {
		t.Fatal("request should be allowed after previous window weight decays")
	}
}

func TestMemoryLimiterCloseConcurrent(t *testing.T) {
	limiter := NewMemoryLimiter("test", time.Minute)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			limiter.Close()
		}()
	}
	wg.Wait()
}
