# ratelimit

**English** | [简体中文](./README.zh-CN.md)

[![Go Reference](https://pkg.go.dev/badge/github.com/onlomk/ratelimit.svg)](https://pkg.go.dev/github.com/onlomk/ratelimit)
[![CI](https://github.com/onlomk/ratelimit/actions/workflows/ci.yml/badge.svg)](https://github.com/onlomk/ratelimit/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/onlomk/ratelimit)](https://goreportcard.com/report/github.com/onlomk/ratelimit)
[![Go Version](https://img.shields.io/github/go-mod/go-version/onlomk/ratelimit)](https://github.com/onlomk/ratelimit/blob/main/go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

Production-ready rate limiting for Go services. `ratelimit` provides a small, stable API with both in-process memory limiting and Redis-backed distributed limiting.

It is designed for API gateways, HTTP middleware, login protection, tenant quotas, SaaS plans, export jobs, and other production scenarios where limits must be easy to reason about and safe to operate.

```bash
go get github.com/onlomk/ratelimit
```

```go
import "github.com/onlomk/ratelimit"
```

## Why ratelimit?

- **One interface for all backends**: switch between memory and Redis without changing business code.
- **Production-oriented Redis backend**: Lua scripts keep each decision atomic.
- **Fast local fallback**: in-memory limiter for single-node apps, tests, development, or Redis degradation.
- **Multiple algorithms**: token bucket, fixed window, sliding window, and sliding window counter.
- **Middleware friendly**: works naturally in route-level, user-level, IP-level, tenant-level, and API key-level middleware.
- **Safe defaults**: empty key, non-positive limit, or non-positive window means no limit, making optional rules easy to compose.
- **Minimal dependencies**: Redis support uses `github.com/redis/go-redis/v9`; memory mode needs no external service.

## Supported backends

| Backend | Constructor | Best for | Notes |
| --- | --- | --- | --- |
| Memory | `NewMemoryLimiter` | single instance apps, local development, unit tests, fallback | state is local to the current process |
| Redis | `NewRedisLimiter` | multi-instance services, distributed API limits | atomic Lua scripts, supports shared limits |
| Redis Cluster / Ring | `NewRedisLimiterWithClient` | distributed Redis deployments | accepts `redis.Scripter` |

## Supported algorithms

| Algorithm | Constant | Recommended use |
| --- | --- | --- |
| Token Bucket | `TokenBucket` | default choice; smooth average rate with controlled burst |
| Fixed Window | `FixedWindow` | simple per-second / per-minute / per-hour caps |
| Sliding Window | `SlidingWindow` | precise rolling window; higher storage cost |
| Sliding Window Counter | `SlidingWindowCounter` | approximate rolling window with lower overhead |

If `Algorithm` is empty or unknown, `TokenBucket` is used.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/onlomk/ratelimit"
)

func main() {
	limiter := ratelimit.NewMemoryLimiter("rate_limit", 5*time.Minute)
	defer limiter.Close()

	allowed, err := limiter.Allow(context.Background(), ratelimit.Rule{
		Key:       "user:123",
		Limit:     100,
		Burst:     100,
		Window:    time.Minute,
		Algorithm: ratelimit.TokenBucket,
	})
	if err != nil {
		panic(err)
	}
	if !allowed {
		fmt.Println("rate limited")
		return
	}

	fmt.Println("allowed")
}
```

## Core API

```go
type Limiter interface {
	Allow(ctx context.Context, rule Rule) (bool, error)
}
```

```go
type Rule struct {
	Key       string
	Limit     int
	Burst     int
	Window    time.Duration
	Algorithm Algorithm
}
```

| Field | Description |
| --- | --- |
| `Key` | rate limit identity, for example user ID, IP, route, tenant, or API key |
| `Limit` | number of allowed requests per window; `<= 0` means unlimited |
| `Burst` | burst capacity for token bucket; `<= 0` falls back to `Limit` |
| `Window` | time window, such as `time.Second`, `time.Minute`, `time.Hour` |
| `Algorithm` | limiter algorithm; defaults to `TokenBucket` |

## Backend usage

### Memory limiter

Use memory mode for a single-process service, tests, local development, or fallback.

```go
limiter := ratelimit.NewMemoryLimiter(ratelimit.DefaultMemoryLimiterPrefix, 5*time.Minute)
defer limiter.Close()

allowed, err := limiter.Allow(ctx, ratelimit.Rule{
	Key:       "route:/api/orders:user:123",
	Limit:     10,
	Burst:     10,
	Window:    time.Second,
	Algorithm: ratelimit.TokenBucket,
})
```

Memory mode starts a cleanup goroutine. Call `Close()` when the limiter is no longer needed.

### Redis limiter

Use Redis mode when multiple service instances must share the same rate limit state.

```go
import (
	"time"

	"github.com/onlomk/ratelimit"
	"github.com/redis/go-redis/v9"
)

rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
defer rdb.Close()

limiter := ratelimit.NewRedisLimiter(
	rdb,
	ratelimit.DefaultRedisLimiterPrefix,
	50*time.Millisecond,
)

allowed, err := limiter.Allow(ctx, ratelimit.Rule{
	Key:       "ip:127.0.0.1",
	Limit:     60,
	Burst:     60,
	Window:    time.Minute,
	Algorithm: ratelimit.TokenBucket,
})
```

### Redis Cluster / Ring

`NewRedisLimiterWithClient` accepts any go-redis client that implements `redis.Scripter`, including `*redis.ClusterClient` and `*redis.Ring`.

```go
cluster := redis.NewClusterClient(&redis.ClusterOptions{
	Addrs: []string{"127.0.0.1:7000", "127.0.0.1:7001"},
})
defer cluster.Close()

limiter := ratelimit.NewRedisLimiterWithClient(cluster, "rate_limit", 50*time.Millisecond)
```

## Real production scenario: API middleware with route plans

In production, rate limiting is usually not written inside every handler. A common pattern is to define a default policy once, then override it for sensitive or expensive routes.

The example below protects:

- `/api/login`: strict minute-level protection against brute force attempts.
- `/api/search`: second-level protection for bursty endpoints.
- `/api/export`: hour-level quota for expensive jobs.
- `/health`: disabled limit for health checks.

```go
type RouteRule struct {
	Limit     int
	Burst     int
	Window    time.Duration
	Algorithm ratelimit.Algorithm
	Disabled  bool
}

func clientKey(r *http.Request) string {
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return "user:" + userID
	}
	return "ip:" + r.RemoteAddr
}

func RateLimitMiddleware(
	limiter ratelimit.Limiter,
	defaultRule RouteRule,
	routeRules map[string]RouteRule,
	next http.Handler,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg := defaultRule
		if routeRule, ok := routeRules[r.URL.Path]; ok {
			cfg = routeRule
		}

		if cfg.Disabled {
			next.ServeHTTP(w, r)
			return
		}

		allowed, err := limiter.Allow(r.Context(), ratelimit.Rule{
			Key:       "route:" + r.URL.Path + ":" + clientKey(r),
			Limit:     cfg.Limit,
			Burst:     cfg.Burst,
			Window:    cfg.Window,
			Algorithm: cfg.Algorithm,
		})
		if err != nil {
			http.Error(w, "rate limit unavailable", http.StatusServiceUnavailable)
			return
		}
		if !allowed {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
```

```go
defaultRule := RouteRule{
	Limit:     100,
	Burst:     100,
	Window:    time.Minute,
	Algorithm: ratelimit.TokenBucket,
}

routeRules := map[string]RouteRule{
	"/api/login": {
		Limit:     5,
		Burst:     5,
		Window:    time.Minute,
		Algorithm: ratelimit.FixedWindow,
	},
	"/api/search": {
		Limit:     10,
		Burst:     10,
		Window:    time.Second,
		Algorithm: ratelimit.TokenBucket,
	},
	"/api/export": {
		Limit:     3,
		Burst:     3,
		Window:    time.Hour,
		Algorithm: ratelimit.SlidingWindowCounter,
	},
	"/health": {
		Disabled: true,
	},
}
```

This keeps business handlers clean while making limits visible at the routing layer.

## Simpler route usage with policies

For most applications you can keep limit configuration at the routing layer and avoid building `Rule` manually in every handler.

```go
allowed, err := ratelimit.AllowAll(ctx, limiter, "user:123",
	ratelimit.PerSecond(5),
	ratelimit.PerMinute(100),
	ratelimit.PerHour(1000).WithAlgorithm(ratelimit.SlidingWindowCounter),
)
```

Gin users can use the optional middleware package:

```go
import "github.com/onlomk/ratelimit/middleware/ginlimit"

r.Use(ginlimit.New(limiter,
	ginlimit.Default(ratelimit.PerMinute(100)),
	ginlimit.Route("/api/login", ratelimit.PerMinute(5).WithAlgorithm(ratelimit.FixedWindow)),
	ginlimit.Route("/api/search", ratelimit.PerSecond(10)),
	ginlimit.Route("/api/export", ratelimit.PerHour(3).WithAlgorithm(ratelimit.SlidingWindowCounter)),
	ginlimit.NoLimit("/health"),
))
```

This makes common route limits readable while the core package remains framework-agnostic.

## Combining multiple windows

Some endpoints need more than one rule. For example, an endpoint may allow 5 requests per second, 100 requests per minute, and 1,000 requests per hour. Check multiple rules and reject when any one fails.

```go
func AllowAll(ctx context.Context, limiter ratelimit.Limiter, rules ...ratelimit.Rule) (bool, error) {
	for _, rule := range rules {
		allowed, err := limiter.Allow(ctx, rule)
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

allowed, err := AllowAll(ctx, limiter,
	ratelimit.Rule{Key: "search:user:123:sec", Limit: 5, Burst: 5, Window: time.Second},
	ratelimit.Rule{Key: "search:user:123:min", Limit: 100, Burst: 100, Window: time.Minute},
	ratelimit.Rule{Key: "search:user:123:hour", Limit: 1000, Burst: 1000, Window: time.Hour, Algorithm: ratelimit.SlidingWindowCounter},
)
```

Use distinct key suffixes such as `:sec`, `:min`, and `:hour` so Redis keys and metrics stay easy to inspect.

## Redis failure fallback

For production services, Redis errors should be handled explicitly. A common policy is to log the error and fall back to memory mode for temporary protection.

```go
type FallbackLimiter struct {
	primary  ratelimit.Limiter
	fallback ratelimit.Limiter
}

func (l FallbackLimiter) Allow(ctx context.Context, rule ratelimit.Rule) (bool, error) {
	allowed, err := l.primary.Allow(ctx, rule)
	if err == nil {
		return allowed, nil
	}

	// Log err in real services, then fall back to local protection.
	return l.fallback.Allow(ctx, rule)
}
```

Memory fallback does not share state across instances, so it should be treated as a temporary safety net rather than a replacement for distributed limiting.

## Key design examples

| Use case | Key example |
| --- | --- |
| IP based | `ip:203.0.113.10` |
| User based | `user:123` |
| Route + user | `route:/api/orders:user:123` |
| Tenant quota | `tenant:acme` |
| SaaS plan | `plan:pro:user:123` |
| API key | `api_key:sha256:...` |

Avoid putting raw tokens, passwords, phone numbers, emails, or private data directly in keys. Hash sensitive identifiers first.

## Benchmarks

Run benchmarks:

```bash
go test -bench=. -benchmem ./...
```

Memory benchmarks cover the common local paths: token bucket, fixed window, sliding window counter, and concurrent access. Redis performance depends on network latency, Redis deployment, and pipelining strategy, so Redis integration tests are kept opt-in.

## Examples

Runnable examples are available in [`examples`](./examples):

- [`examples/memory`](./examples/memory): basic in-process limiter usage.
- [`examples/http_middleware`](./examples/http_middleware): production-style `net/http` middleware with route-level policies.
- [`examples/fallback`](./examples/fallback): Redis primary limiter with memory fallback.
- [`examples/gin`](./examples/gin): Gin middleware with default rules and route-level overrides.

## Project docs

- [CHANGELOG](./CHANGELOG.md)
- [CONTRIBUTING](./CONTRIBUTING.md)
- [SECURITY](./SECURITY.md)
- [CODE_OF_CONDUCT](./CODE_OF_CONDUCT.md)

## Tests

Run unit tests:

```bash
go test ./...
```

Redis script integration tests are skipped by default. Set `RATELIMIT_REDIS_ADDR` to run them against a real Redis instance:

```bash
RATELIMIT_REDIS_ADDR=127.0.0.1:6379 go test ./...
```

The Redis tests use a unique key prefix and only clean up keys under that prefix.

## Compatibility

The following aliases are kept for existing users:

- `RedisRule` is an alias of `Rule`
- `RedisAlgorithm` is an alias of `Algorithm`
- `RedisTokenBucket`, `RedisFixedWindow`, `RedisSlidingWindow`, `RedisSlidingWindowCounter`

## License

MIT License. See [LICENSE](./LICENSE).
