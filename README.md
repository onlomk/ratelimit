# ratelimit

`ratelimit` 是一个轻量级 Go 限流包，提供统一的 `Limiter` 接口，支持 Redis 分布式限流和进程内内存限流。适合直接作为业务项目内
pkg 使用，也方便拆分成独立开源包。

## 支持能力

### 后端支持

| 后端     | 构造函数                                            | 适用场景                     | 说明                 |
|--------|-------------------------------------------------|--------------------------|--------------------|
| Redis  | `NewRedisLimiter` / `NewRedisLimiterWithClient` | 多实例服务、分布式限流              | 使用 Lua 脚本保证单次判断原子性 |
| Memory | `NewMemoryLimiter`                              | 单实例服务、本地开发、单元测试、Redis 降级 | 状态只在当前进程内生效        |

### 算法支持

| 算法      | 常量                     | 适用场景                                       |
|---------|------------------------|--------------------------------------------|
| 令牌桶     | `TokenBucket`          | 默认推荐，允许短时间突发，长期速率受控                        |
| 固定窗口    | `FixedWindow`          | 实现简单，适合粗粒度窗口限制                             |
| 滑动窗口    | `SlidingWindow`        | 精确滑动窗口；Redis 使用 sorted set，高基数 key 下资源占用更高 |
| 滑动窗口计数器 | `SlidingWindowCounter` | 近似滑动窗口，资源占用较低                              |

未设置 `Algorithm` 或设置未知值时，默认使用 `TokenBucket`。

### 兼容性

保留以下历史名称，已有代码无需立即迁移：

- `RedisRule` 等价于 `Rule`
- `RedisAlgorithm` 等价于 `Algorithm`
- `RedisTokenBucket` / `RedisFixedWindow` / `RedisSlidingWindow` / `RedisSlidingWindowCounter`

## 安装 / 引入

当前包位于项目内：

```go
import "go-api/pkg/ratelimit"
```

如果拆分为独立开源包，只需要替换为实际 module path。

Redis 后端需要 go-redis：

```go
import "github.com/redis/go-redis/v9"
```

## 核心接口

```go
type Limiter interface {
Allow(ctx context.Context, rule Rule) (bool, error)
}
```

`Rule` 字段说明：

| 字段          | 说明                          |
|-------------|-----------------------------|
| `Key`       | 限流对象，例如 IP、用户 ID、API 路由等    |
| `Limit`     | 一个窗口内允许的请求数；非正数表示不限流        |
| `Burst`     | 突发容量，主要用于令牌桶；非正数时使用 `Limit` |
| `Window`    | 限流窗口；非正数表示不限流               |
| `Algorithm` | 限流算法；为空时默认 `TokenBucket`    |

`Key` 为空、`Limit <= 0` 或 `Window <= 0` 时，会被视为不限流并直接允许。

## 快速开始

```go
package main

import (
	"context"
	"fmt"
	"time"

	"go-api/pkg/ratelimit"
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

## Redis 用法

### 普通 Redis Client

```go
package main

import (
	"context"
	"time"

	"go-api/pkg/ratelimit"
)

func main() {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
	defer rdb.Close()

	limiter := ratelimit.NewRedisLimiter(
		rdb,
		ratelimit.DefaultRedisLimiterPrefix,
		50*time.Millisecond,
	)

	allowed, err := limiter.Allow(context.Background(), ratelimit.Rule{
		Key:       "ip:127.0.0.1",
		Limit:     60,
		Burst:     60,
		Window:    time.Minute,
		Algorithm: ratelimit.TokenBucket,
	})
	if err != nil {
		// Redis 网络错误、超时、脚本执行错误等。
		// 生产环境可选择降级到内存限流或直接拒绝请求。
		return
	}
	if !allowed {
		// reject request
		return
	}
}

```

### Cluster / Ring / 自定义 go-redis 客户端

`NewRedisLimiterWithClient` 接收 `redis.Scripter`，可用于 `*redis.ClusterClient`、`*redis.Ring` 等支持脚本执行的客户端。

```go
cluster := redis.NewClusterClient(&redis.ClusterOptions{
Addrs: []string{"127.0.0.1:7000", "127.0.0.1:7001"},
})
defer cluster.Close()

limiter := ratelimit.NewRedisLimiterWithClient(cluster, "rate_limit", 50*time.Millisecond)
```

## 内存用法

```go
limiter := ratelimit.NewMemoryLimiter(ratelimit.DefaultMemoryLimiterPrefix, 5*time.Minute)
defer limiter.Close()

allowed, err := limiter.Allow(ctx, ratelimit.Rule{
	Key:       "route:/api/orders:user:123",
	Limit:     10,
	Window:    time.Second,
	Algorithm: ratelimit.FixedWindow,
})
if err != nil {
	return err
}
if !allowed {
	// reject request
}
```

内存后端会启动后台清理 goroutine，建议在生命周期结束时调用 `Close()`。

## 秒 / 分钟 / 小时限流示例

`Window` 使用 Go 标准库 `time.Duration`，因此天然支持秒、分钟、小时，也支持任意自定义时间窗口。

### 每秒限流

适合保护高频接口，例如短信验证码、登录尝试、搜索接口等。

```go
rule := ratelimit.Rule{
    Key:       "user:123:sms",
    Limit:     1,
    Burst:     1,
    Window:    time.Second,
    Algorithm: ratelimit.TokenBucket,
}

allowed, err := limiter.Allow(ctx, rule)
```

含义：同一个 `Key` 平均每秒允许 1 次请求。

### 每分钟限流

适合常见 API 访问限制。

```go
rule := ratelimit.Rule{
    Key:       "ip:127.0.0.1",
    Limit:     60,
    Burst:     60,
    Window:    time.Minute,
    Algorithm: ratelimit.TokenBucket,
}

allowed, err := limiter.Allow(ctx, rule)
```

含义：同一个 `Key` 平均每分钟允许 60 次请求。

### 每小时限流

适合导出、上传、批量操作等低频重操作。

```go
rule := ratelimit.Rule{
    Key:       "user:123:export",
    Limit:     10,
    Burst:     10,
    Window:    time.Hour,
    Algorithm: ratelimit.FixedWindow,
}

allowed, err := limiter.Allow(ctx, rule)
```

含义：同一个 `Key` 每小时最多允许 10 次请求。

### 自定义时间窗口

```go
rule := ratelimit.Rule{
    Key:       "api_key:abc",
    Limit:     300,
    Burst:     300,
    Window:    5 * time.Minute,
    Algorithm: ratelimit.SlidingWindowCounter,
}

allowed, err := limiter.Allow(ctx, rule)
```

含义：同一个 `Key` 在 5 分钟窗口内允许约 300 次请求。

### 同时设置秒 / 分钟 / 小时多种规则

有些接口既要限制瞬时突发，也要限制中长期总量。可以为同一个请求同时检查多条规则，只要任意一条不通过就拒绝。

```go
func allowWithRules(ctx context.Context, limiter ratelimit.Limiter, rules ...ratelimit.Rule) (bool, error) {
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

rules := []ratelimit.Rule{
    // 秒级：限制瞬时突发，最多每秒 5 次。
    {
        Key:       "route:/api/search:user:123:second",
        Limit:     5,
        Burst:     5,
        Window:    time.Second,
        Algorithm: ratelimit.TokenBucket,
    },
    // 分钟级：限制常规访问量，最多每分钟 100 次。
    {
        Key:       "route:/api/search:user:123:minute",
        Limit:     100,
        Burst:     100,
        Window:    time.Minute,
        Algorithm: ratelimit.TokenBucket,
    },
    // 小时级：限制长期总量，最多每小时 1000 次。
    {
        Key:       "route:/api/search:user:123:hour",
        Limit:     1000,
        Burst:     1000,
        Window:    time.Hour,
        Algorithm: ratelimit.SlidingWindowCounter,
    },
}

allowed, err := allowWithRules(ctx, limiter, rules...)
if err != nil {
    return err
}
if !allowed {
    // reject request
}
```

建议为不同窗口使用不同 `Key` 后缀，例如 `:second`、`:minute`、`:hour`，便于 Redis key 排查和指标统计。

## 自定义规则使用

实际业务中通常会根据用户、路由、角色、套餐或租户动态生成 `Rule`。

### 按路由自定义规则

```go
func ruleForPath(path string, userID string) ratelimit.Rule {
    switch path {
    case "/api/login":
        return ratelimit.Rule{
            Key:       "login:user:" + userID,
            Limit:     5,
            Burst:     5,
            Window:    time.Minute,
            Algorithm: ratelimit.FixedWindow,
        }
    case "/api/export":
        return ratelimit.Rule{
            Key:       "export:user:" + userID,
            Limit:     3,
            Burst:     3,
            Window:    time.Hour,
            Algorithm: ratelimit.SlidingWindowCounter,
        }
    default:
        return ratelimit.Rule{
            Key:       "default:user:" + userID,
            Limit:     100,
            Burst:     100,
            Window:    time.Minute,
            Algorithm: ratelimit.TokenBucket,
        }
    }
}
```

### 按用户等级自定义规则

```go
func ruleForPlan(userID string, plan string) ratelimit.Rule {
    limit := 100
    burst := 100

    switch plan {
    case "free":
        limit = 60
        burst = 60
    case "pro":
        limit = 600
        burst = 600
    case "enterprise":
        limit = 3000
        burst = 3000
    }

    return ratelimit.Rule{
        Key:       "plan:" + plan + ":user:" + userID,
        Limit:     limit,
        Burst:     burst,
        Window:    time.Minute,
        Algorithm: ratelimit.TokenBucket,
    }
}
```

### 在请求处理中使用自定义规则

```go
rule := ruleForPath(r.URL.Path, userID)

allowed, err := limiter.Allow(r.Context(), rule)
if err != nil {
    http.Error(w, "rate limit unavailable", http.StatusServiceUnavailable)
    return
}
if !allowed {
    http.Error(w, "too many requests", http.StatusTooManyRequests)
    return
}
```

### 不限流规则

当业务希望某些用户或路由跳过限流时，可以返回非正数 `Limit` 或空 `Key`。

```go
rule := ratelimit.Rule{
    Key:    "admin:user:1",
    Limit:  0,
    Window: time.Minute,
}
```

上面规则会直接允许请求，不产生限流效果。

### 清除 / 禁用默认规则

如果业务有“默认规则”，但某些路由、租户或用户需要清除默认限流，可以返回不限流规则覆盖默认值。

方式一：设置 `Limit <= 0`。

```go
func ruleForPath(path string, userID string) ratelimit.Rule {
    if path == "/api/health" || path == "/metrics" {
        return ratelimit.Rule{
            Key:    "",
            Limit:  0,
            Window: 0,
        }
    }

    return ratelimit.Rule{
        Key:       "default:user:" + userID,
        Limit:     100,
        Burst:     100,
        Window:    time.Minute,
        Algorithm: ratelimit.TokenBucket,
    }
}
```

方式二：配置多规则时，使用 `nil` 或空切片表示不应用默认规则。

```go
func rulesForPath(path string, userID string) []ratelimit.Rule {
    switch path {
    case "/api/health", "/metrics":
        // 清除默认设置：不返回任何规则，即不执行限流。
        return nil
    case "/api/export":
        return []ratelimit.Rule{
            {
                Key:       "export:user:" + userID + ":hour",
                Limit:     3,
                Burst:     3,
                Window:    time.Hour,
                Algorithm: ratelimit.SlidingWindowCounter,
            },
        }
    default:
        return []ratelimit.Rule{
            {
                Key:       "default:user:" + userID + ":minute",
                Limit:     100,
                Burst:     100,
                Window:    time.Minute,
                Algorithm: ratelimit.TokenBucket,
            },
        }
    }
}
```

注意：`ratelimit` 包本身不保存全局默认规则，默认规则通常由业务层配置并传入 `Allow`。因此“清除默认设置”的本质是业务层不要调用默认规则，或传入 `Limit <= 0` / 空 `Key` 的不限流规则。

## Redis 失败时降级到内存

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

	// 这里可以记录 Redis 错误日志，再使用内存限流兜底。
	return l.fallback.Allow(ctx, rule)
}

func NewLimiter(rdb *redis.Client) ratelimit.Limiter {
	return FallbackLimiter{
		primary:  ratelimit.NewRedisLimiter(rdb, "rate_limit", 50*time.Millisecond),
		fallback: ratelimit.NewMemoryLimiter("rate_limit:fallback", 5*time.Minute),
	}
}
```

注意：降级到内存后，多实例之间不再共享限流状态，只能作为临时保护手段。

## HTTP 中间件示例

以下示例使用标准库 `net/http`，也可按同样方式接入 Gin、Echo、Fiber 等框架。

```go
func RateLimitMiddleware(limiter ratelimit.Limiter, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.RemoteAddr
		if userID := r.Header.Get("X-User-ID"); userID != "" {
			key = "user:" + userID
		}

		allowed, err := limiter.Allow(r.Context(), ratelimit.Rule{
			Key:       key,
			Limit:     100,
			Burst:     100,
			Window:    time.Minute,
			Algorithm: ratelimit.TokenBucket,
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

## 推荐：在路由 / 中间件中统一封装

限流通常不建议在每个 handler 里手写 `Rule`，否则配置会很分散。更推荐在路由层或中间件层统一封装：

- 全局默认限流：大部分接口共用一套规则。
- 路由级覆盖：少数接口单独设置秒 / 分钟 / 小时规则。
- 白名单 / 不限流：健康检查、监控、内部接口跳过限流。

### 简化版中间件：只配置一次默认规则

```go
func NewRateLimitMiddleware(limiter ratelimit.Limiter, defaultRule ratelimit.Rule) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rule := defaultRule
			rule.Key = clientKey(r)

			allowed, err := limiter.Allow(r.Context(), rule)
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
}

func clientKey(r *http.Request) string {
	if userID := r.Header.Get("X-User-ID"); userID != "" {
		return "user:" + userID
	}
	return "ip:" + r.RemoteAddr
}
```

使用时只需要在路由注册处设置一次：

```go
limiter := ratelimit.NewMemoryLimiter("rate_limit", 5*time.Minute)
defer limiter.Close()

rateLimit := NewRateLimitMiddleware(limiter, ratelimit.Rule{
	Limit:     100,
	Burst:     100,
	Window:    time.Minute,
	Algorithm: ratelimit.TokenBucket,
})

mux := http.NewServeMux()
mux.Handle("/api/orders", rateLimit(http.HandlerFunc(orderHandler)))
mux.Handle("/api/users", rateLimit(http.HandlerFunc(userHandler)))
```

### 路由级规则：少数接口覆盖默认值

如果不同接口需要不同规则，可以维护一张路由规则表，中间件自动按路径选择规则。

```go
type RouteRule struct {
	Limit     int
	Burst     int
	Window    time.Duration
	Algorithm ratelimit.Algorithm
	Disabled  bool
}

func NewRouteRateLimitMiddleware(
	limiter ratelimit.Limiter,
	defaultRule RouteRule,
	routeRules map[string]RouteRule,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
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
}
```

配置示例：

```go
defaultRule := RouteRule{
	Limit:     100,
	Burst:     100,
	Window:    time.Minute,
	Algorithm: ratelimit.TokenBucket,
}

routeRules := map[string]RouteRule{
	// 登录接口：每分钟 5 次。
	"/api/login": {
		Limit:     5,
		Burst:     5,
		Window:    time.Minute,
		Algorithm: ratelimit.FixedWindow,
	},
	// 搜索接口：每秒 10 次。
	"/api/search": {
		Limit:     10,
		Burst:     10,
		Window:    time.Second,
		Algorithm: ratelimit.TokenBucket,
	},
	// 导出接口：每小时 3 次。
	"/api/export": {
		Limit:     3,
		Burst:     3,
		Window:    time.Hour,
		Algorithm: ratelimit.SlidingWindowCounter,
	},
	// 健康检查：清除默认限流，不限流。
	"/health": {
		Disabled: true,
	},
}

rateLimit := NewRouteRateLimitMiddleware(limiter, defaultRule, routeRules)
```

这样业务 handler 不需要关心限流规则，只在路由配置处维护默认规则和少量覆盖规则。

## 常见限流 Key 设计

| 场景           | Key 示例                       |
|--------------|------------------------------|
| 按 IP 限流      | `ip:127.0.0.1`               |
| 按用户限流        | `user:123`                   |
| 按路由 + 用户限流   | `route:/api/orders:user:123` |
| 按租户限流        | `tenant:abc`                 |
| 按 API Key 限流 | `api_key:xxxxx`              |

建议 key 中不要包含明文密钥、Token、手机号、邮箱等敏感信息；如必须使用，可先做哈希。

## 测试

运行基础测试：

```bash
go test ./pkg/ratelimit
```

Redis Lua 脚本测试默认跳过。设置 `RATELIMIT_REDIS_ADDR` 后会连接真实 Redis 执行集成测试：

```bash
RATELIMIT_REDIS_ADDR=127.0.0.1:6379 go test ./pkg/ratelimit
```

测试会使用唯一 key 前缀，并在结束时只清理该前缀下的 key。

## 注意事项

- Redis 后端适合分布式限流；内存后端只适合单进程限流。
- Redis 错误应由调用方按业务策略处理，例如降级、拒绝或放行。
- `SlidingWindow` 更精确，但保存窗口内请求记录；高并发、高基数 key 下要关注 Redis 内存占用。
- `TokenBucket` 一般作为默认算法更稳妥，兼顾突发流量和平均速率。
