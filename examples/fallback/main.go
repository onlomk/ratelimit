package main

import (
	"context"
	"fmt"
	"time"

	"github.com/onlomk/ratelimit"
	"github.com/redis/go-redis/v9"
)

type FallbackLimiter struct {
	primary  ratelimit.Limiter
	fallback ratelimit.Limiter
}

func (l FallbackLimiter) Allow(ctx context.Context, rule ratelimit.Rule) (bool, error) {
	allowed, err := l.primary.Allow(ctx, rule)
	if err == nil {
		return allowed, nil
	}

	// Log err in production, then continue with local protection.
	return l.fallback.Allow(ctx, rule)
}

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	defer rdb.Close()

	fallback := ratelimit.NewMemoryLimiter("fallback", 5*time.Minute)
	defer fallback.Close()

	limiter := FallbackLimiter{
		primary:  ratelimit.NewRedisLimiter(rdb, "rate_limit", 50*time.Millisecond),
		fallback: fallback,
	}

	allowed, err := limiter.Allow(context.Background(), ratelimit.Rule{
		Key:       "api:user:123",
		Limit:     100,
		Burst:     100,
		Window:    time.Minute,
		Algorithm: ratelimit.TokenBucket,
	})
	if err != nil {
		panic(err)
	}

	fmt.Println("allowed:", allowed)
}
