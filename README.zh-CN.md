# ratelimit

[English](./README.md) | **简体中文**

[![Go Reference](https://pkg.go.dev/badge/github.com/onlomk/ratelimit.svg)](https://pkg.go.dev/github.com/onlomk/ratelimit)
[![Go Report Card](https://goreportcard.com/badge/github.com/onlomk/ratelimit)](https://goreportcard.com/report/github.com/onlomk/ratelimit)
[![Go Version](https://img.shields.io/github/go-mod/go-version/onlomk/ratelimit)](https://github.com/onlomk/ratelimit/blob/main/go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](./LICENSE)

面向生产环境的 Go 限流组件。`ratelimit` 提供一套小而稳定的 API，同时支持进程内内存限流和 Redis 分布式限流。

它适合 API 网关、HTTP 中间件、登录保护、租户配额、SaaS 套餐、导出任务等真实生产场景，目标是让限流规则易于理解、易于维护、可安全运行。

```bash
go get github.com/onlomk/ratelimit
```

```go
import "github.com/onlomk/ratelimit"
```

## 为什么选择 ratelimit？

- **统一接口**：业务代码只依赖 `Limiter`，可在内存和 Redis 后端之间切换。
- **生产级 Redis 后端**：使用 Lua 脚本保证单次限流判断原子性。
- **快速本地兜底**：内存限流适合单实例、测试、本地开发，或 Redis 降级保护。
- **多种算法**：支持令牌桶、固定窗口、滑动窗口、滑动窗口计数器。
- **适合中间件封装**：天然适配路由级、用户级、IP 级、租户级、API Key 级限流。
- **安全默认行为**：空 key、非正数 limit、非正数 window 会被视为不限流，便于组合可选规则。
- **依赖少**：Redis 后端使用 `github.com/redis/go-redis/v9`；内存模式不需要外部服务。

## 支持的后端

| 后端 | 构造函数 | 适合场景 | 说明 |
| --- | --- | --- | --- |
| Memory | `NewMemoryLimiter` | 单实例应用、本地开发、单元测试、降级兜底 | 状态只保存在当前进程 |
| Redis | `NewRedisLimiter` | 多实例服务、分布式 API 限流 | Lua 脚本原子执行，共享限流状态 |
| Redis Cluster / Ring | `NewRedisLimiterWithClient` | Redis 集群或 Ring 部署 | 接收 `redis.Scripter` |

## 支持的算法

| 算法 | 常量 | 推荐场景 |
| --- | --- | --- |
| 令牌桶 | `TokenBucket` | 默认推荐；允许可控突发，同时限制平均速率 |
| 固定窗口 | `FixedWindow` | 简单的秒级 / 分钟级 / 小时级上限 |
| 滑动窗口 | `SlidingWindow` | 精确滚动窗口；存储成本更高 |
| 滑动窗口计数器 | `SlidingWindowCounter` | 近似滚动窗口；开销更低 |

`Algorithm` 为空或未知时，默认使用 `TokenBucket`。

## 快速开始

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

## 核心 API

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

| 字段 | 说明 |
| --- | --- |
| `Key` | 限流对象，例如用户 ID、IP、路由、租户、API Key |
| `Limit` | 窗口内允许的请求数；`<= 0` 表示不限流 |
| `Burst` | 令牌桶突发容量；`<= 0` 时使用 `Limit` |
| `Window` | 限流窗口，例如 `time.Second`、`time.Minute`、`time.Hour` |
| `Algorithm` | 限流算法；默认 `TokenBucket` |

## 后端用法

### 内存限流

内存模式适合单进程服务、测试、本地开发，或作为 Redis 失败时的降级保护。

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

内存模式会启动后台清理 goroutine，生命周期结束时建议调用 `Close()`。

### Redis 限流

当多个服务实例需要共享限流状态时，推荐使用 Redis 后端。

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

`NewRedisLimiterWithClient` 接收任何实现 `redis.Scripter` 的 go-redis 客户端，包括 `*redis.ClusterClient` 和 `*redis.Ring`。

```go
cluster := redis.NewClusterClient(&redis.ClusterOptions{
	Addrs: []string{"127.0.0.1:7000", "127.0.0.1:7001"},
})
defer cluster.Close()

limiter := ratelimit.NewRedisLimiterWithClient(cluster, "rate_limit", 50*time.Millisecond)
```

## 真实生产场景：按路由配置限流策略

生产环境中，限流通常不应该写在每个 handler 里。更常见的方式是在中间件里定义默认策略，再对敏感或高成本接口做覆盖。

下面的例子保护了：

- `/api/login`：分钟级严格限制，防止暴力破解。
- `/api/search`：秒级限制，保护高频查询接口。
- `/api/export`：小时级配额，限制高成本导出任务。
- `/health`：关闭限流，供健康检查使用。

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

这样业务 handler 不需要关心限流，所有规则都集中在路由层，便于审查和调整。

## 组合多个时间窗口

有些接口需要同时限制短期突发和长期总量。例如：每秒 5 次、每分钟 100 次、每小时 1000 次。可以连续检查多条规则，任意一条失败就拒绝。

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

建议使用 `:sec`、`:min`、`:hour` 等不同 key 后缀，方便排查 Redis key 和统计指标。

## Redis 失败降级

生产服务应显式处理 Redis 错误。常见策略是记录日志，并临时降级到内存限流。

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

	// 真实服务中应记录 err，然后使用本地保护兜底。
	return l.fallback.Allow(ctx, rule)
}
```

内存降级无法在多实例间共享状态，因此它更适合作为临时安全网，而不是分布式限流替代品。

## Key 设计示例

| 场景 | Key 示例 |
| --- | --- |
| 按 IP | `ip:203.0.113.10` |
| 按用户 | `user:123` |
| 路由 + 用户 | `route:/api/orders:user:123` |
| 租户配额 | `tenant:acme` |
| SaaS 套餐 | `plan:pro:user:123` |
| API Key | `api_key:sha256:...` |

不要把原始 token、密码、手机号、邮箱或隐私数据直接放进 key。必要时先做哈希。

## Benchmark

运行基准测试：

```bash
go test -bench=. -benchmem ./...
```

内存 benchmark 覆盖常见本地路径：令牌桶、固定窗口、滑动窗口计数器、高基数 key、并发访问。Redis 性能取决于网络延迟、Redis 部署和调用策略，因此 Redis 集成测试默认保持可选。

## Examples 示例

可运行示例位于 [`examples`](./examples)：

- [`examples/memory`](./examples/memory)：基础进程内内存限流用法。
- [`examples/http_middleware`](./examples/http_middleware)：生产风格 `net/http` 中间件，支持路由级策略。
- [`examples/fallback`](./examples/fallback)：Redis 主限流器 + 内存降级兜底。

## 测试

运行单元测试：

```bash
go test ./...
```

Redis 脚本集成测试默认跳过。设置 `RATELIMIT_REDIS_ADDR` 后会连接真实 Redis 执行：

```bash
RATELIMIT_REDIS_ADDR=127.0.0.1:6379 go test ./...
```

Redis 测试会使用唯一 key 前缀，并只清理该前缀下的 key。

## 兼容性

保留以下别名，兼容已有用户：

- `RedisRule` 是 `Rule` 的别名
- `RedisAlgorithm` 是 `Algorithm` 的别名
- `RedisTokenBucket`、`RedisFixedWindow`、`RedisSlidingWindow`、`RedisSlidingWindowCounter`

## License

MIT License. See [LICENSE](./LICENSE).
