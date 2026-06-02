package ratelimit

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisLimiterScripts(t *testing.T) {
	addr := os.Getenv("RATELIMIT_REDIS_ADDR")
	if addr == "" {
		t.Skip("set RATELIMIT_REDIS_ADDR to run Redis script integration tests")
	}

	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })

	if err := client.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping redis: %v", err)
	}

	prefix := fmt.Sprintf("ratelimit:test:%d", time.Now().UnixNano())
	t.Cleanup(func() { deleteRedisKeysByPrefix(ctx, t, client, prefix) })

	limiter := NewRedisLimiter(client, prefix, time.Second)
	tests := []struct {
		name      string
		algorithm Algorithm
	}{
		{name: "fixed_window", algorithm: FixedWindow},
		{name: "token_bucket", algorithm: TokenBucket},
		{name: "sliding_window", algorithm: SlidingWindow},
		{name: "sliding_window_counter", algorithm: SlidingWindowCounter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := Rule{
				Key:       tt.name,
				Limit:     2,
				Burst:     2,
				Window:    time.Minute,
				Algorithm: tt.algorithm,
			}

			for i := 0; i < 2; i++ {
				allowed, err := limiter.Allow(ctx, rule)
				if err != nil {
					t.Fatalf("allow request %d: %v", i+1, err)
				}
				if !allowed {
					t.Fatalf("request %d should be allowed", i+1)
				}
			}

			allowed, err := limiter.Allow(ctx, rule)
			if err != nil {
				t.Fatalf("allow rejected request: %v", err)
			}
			if allowed {
				t.Fatal("third request should be rejected")
			}
		})
	}
}

func TestRedisLimiterNilClient(t *testing.T) {
	limiter := NewRedisLimiter(nil, "", 0)
	allowed, err := limiter.Allow(context.Background(), Rule{Key: "client", Limit: 1, Window: time.Second})
	if err != ErrNilRedisClient {
		t.Fatalf("expected ErrNilRedisClient, got allowed=%v err=%v", allowed, err)
	}
	if allowed {
		t.Fatal("nil redis client should not allow a valid limited rule")
	}
}

func TestRedisClusterKeysShareHashTag(t *testing.T) {
	slidingKey := redisClusterKey("rate_limit", "sliding", "client:123", "")
	seqKey := slidingKey + ":seq"
	if slidingKey != "rate_limit:sliding:{client:123}" {
		t.Fatalf("unexpected sliding key: %q", slidingKey)
	}
	if seqKey != "rate_limit:sliding:{client:123}:seq" {
		t.Fatalf("unexpected sequence key: %q", seqKey)
	}

	currentKey := redisClusterKey("rate_limit", "sliding_counter", "client:123", "100")
	previousKey := redisClusterKey("rate_limit", "sliding_counter", "client:123", "99")
	if currentKey != "rate_limit:sliding_counter:{client:123}:100" {
		t.Fatalf("unexpected current key: %q", currentKey)
	}
	if previousKey != "rate_limit:sliding_counter:{client:123}:99" {
		t.Fatalf("unexpected previous key: %q", previousKey)
	}
}

func deleteRedisKeysByPrefix(ctx context.Context, t *testing.T, client *redis.Client, prefix string) {
	t.Helper()

	var cursor uint64
	pattern := prefix + "*"
	for {
		keys, next, err := client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			t.Logf("scan redis keys for cleanup: %v", err)
			return
		}
		if len(keys) > 0 {
			if err := client.Del(ctx, keys...).Err(); err != nil {
				t.Logf("delete redis keys for cleanup: %v", err)
				return
			}
		}
		if next == 0 {
			return
		}
		cursor = next
	}
}
