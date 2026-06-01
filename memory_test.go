package ratelimit

import (
	"context"
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
