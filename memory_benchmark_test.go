package ratelimit

import (
	"context"
	"strconv"
	"testing"
	"time"
)

func BenchmarkMemoryLimiterTokenBucket(b *testing.B) {
	limiter := NewMemoryLimiter("bench", time.Minute)
	defer limiter.Close()

	rule := Rule{
		Key:       "user:token-bucket",
		Limit:     b.N + 1,
		Burst:     b.N + 1,
		Window:    time.Minute,
		Algorithm: TokenBucket,
	}

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		allowed, err := limiter.Allow(ctx, rule)
		if err != nil || !allowed {
			b.Fatalf("Allow() allowed=%v err=%v", allowed, err)
		}
	}
}

func BenchmarkMemoryLimiterFixedWindow(b *testing.B) {
	limiter := NewMemoryLimiter("bench", time.Minute)
	defer limiter.Close()

	rule := Rule{
		Key:       "user:fixed-window",
		Limit:     b.N + 1,
		Window:    time.Minute,
		Algorithm: FixedWindow,
	}

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		allowed, err := limiter.Allow(ctx, rule)
		if err != nil || !allowed {
			b.Fatalf("Allow() allowed=%v err=%v", allowed, err)
		}
	}
}

func BenchmarkMemoryLimiterSlidingWindowCounter(b *testing.B) {
	limiter := NewMemoryLimiter("bench", time.Minute)
	defer limiter.Close()

	rule := Rule{
		Key:       "user:sliding-counter",
		Limit:     b.N + 1,
		Window:    time.Minute,
		Algorithm: SlidingWindowCounter,
	}

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		allowed, err := limiter.Allow(ctx, rule)
		if err != nil || !allowed {
			b.Fatalf("Allow() allowed=%v err=%v", allowed, err)
		}
	}
}

func BenchmarkMemoryLimiterHighCardinalityKeys(b *testing.B) {
	limiter := NewMemoryLimiter("bench", time.Minute)
	defer limiter.Close()

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rule := Rule{
			Key:       "user:" + strconv.Itoa(i),
			Limit:     100,
			Burst:     100,
			Window:    time.Minute,
			Algorithm: TokenBucket,
		}
		allowed, err := limiter.Allow(ctx, rule)
		if err != nil || !allowed {
			b.Fatalf("Allow() allowed=%v err=%v", allowed, err)
		}
	}
}

func BenchmarkMemoryLimiterParallel(b *testing.B) {
	limiter := NewMemoryLimiter("bench", time.Minute)
	defer limiter.Close()

	rule := Rule{
		Key:       "user:parallel",
		Limit:     b.N + 1,
		Burst:     b.N + 1,
		Window:    time.Minute,
		Algorithm: TokenBucket,
	}

	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			allowed, err := limiter.Allow(ctx, rule)
			if err != nil || !allowed {
				b.Fatalf("Allow() allowed=%v err=%v", allowed, err)
			}
		}
	})
}
