package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestPolicyRule(t *testing.T) {
	policy := PerMinute(100).WithBurst(20).WithAlgorithm(FixedWindow)
	rule := policy.Rule("user:123")

	if rule.Key != "user:123" {
		t.Fatalf("unexpected key: %q", rule.Key)
	}
	if rule.Limit != 100 || rule.Burst != 20 || rule.Window != time.Minute || rule.Algorithm != FixedWindow {
		t.Fatalf("unexpected rule: %+v", rule)
	}
}

func TestAllowAll(t *testing.T) {
	limiter := NewMemoryLimiter("test", time.Minute)
	defer limiter.Close()

	allowed, err := AllowAll(context.Background(), limiter, "user:123", PerSecond(1), PerMinute(10))
	if err != nil {
		t.Fatalf("AllowAll returned error: %v", err)
	}
	if !allowed {
		t.Fatal("first request should be allowed")
	}

	allowed, err = AllowAll(context.Background(), limiter, "user:123", PerSecond(1), PerMinute(10))
	if err != nil {
		t.Fatalf("AllowAll returned error: %v", err)
	}
	if allowed {
		t.Fatal("second request should be rejected by per-second policy")
	}
}

func TestAllowAllScopesMultiplePolicies(t *testing.T) {
	limiter := NewMemoryLimiter("test", time.Minute)
	defer limiter.Close()

	for i := 0; i < 2; i++ {
		allowed, err := AllowAll(context.Background(), limiter, "user:123", PerSecond(2), PerMinute(10))
		if err != nil {
			t.Fatalf("AllowAll returned error: %v", err)
		}
		if !allowed {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}
