package ratelimit

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Algorithm identifies the rate limiting algorithm used by a Limiter.
type Algorithm string

// RedisAlgorithm is kept for backward compatibility.
type RedisAlgorithm = Algorithm

const (
	FixedWindow          Algorithm = "fixed_window"
	TokenBucket          Algorithm = "token_bucket"
	SlidingWindow        Algorithm = "sliding_window"
	SlidingWindowCounter Algorithm = "sliding_window_counter"

	// Backward-compatible Redis-prefixed names.
	RedisFixedWindow          = FixedWindow
	RedisTokenBucket          = TokenBucket
	RedisSlidingWindow        = SlidingWindow
	RedisSlidingWindowCounter = SlidingWindowCounter
)

const defaultRedisLimiterTimeout = 50 * time.Millisecond

const DefaultRedisLimiterPrefix = "rate_limit"

var ErrNilRedisClient = errors.New("ratelimit: nil redis client")

// Rule describes a single rate limit decision.
//
// Empty keys, non-positive limits, or non-positive windows are treated as
// unlimited to make optional rules easy to wire into middleware.
type Rule struct {
	Key       string
	Limit     int
	Burst     int
	Window    time.Duration
	Algorithm Algorithm
}

// RedisRule is kept for backward compatibility.
type RedisRule = Rule

// Limiter is implemented by RedisLimiter and MemoryLimiter.
type Limiter interface {
	Allow(ctx context.Context, rule Rule) (bool, error)
}

type RedisLimiter struct {
	client  redis.Scripter
	prefix  string
	timeout time.Duration
}

func NewRedisLimiter(client *redis.Client, prefix string, timeout time.Duration) *RedisLimiter {
	if client == nil {
		return NewRedisLimiterWithClient(nil, prefix, timeout)
	}
	return NewRedisLimiterWithClient(client, prefix, timeout)
}

// NewRedisLimiterWithClient creates a Redis-backed limiter from any go-redis
// client that supports script execution, including *redis.Client,
// *redis.ClusterClient, and *redis.Ring.
func NewRedisLimiterWithClient(client redis.Scripter, prefix string, timeout time.Duration) *RedisLimiter {
	if prefix == "" {
		prefix = DefaultRedisLimiterPrefix
	}
	if timeout <= 0 {
		timeout = defaultRedisLimiterTimeout
	}
	return &RedisLimiter{client: client, prefix: prefix, timeout: timeout}
}

func (l *RedisLimiter) Allow(ctx context.Context, rule Rule) (bool, error) {
	if rule.Limit <= 0 || rule.Window <= 0 {
		return true, nil
	}
	if rule.Key == "" {
		return true, nil
	}
	if l == nil || l.client == nil {
		return false, ErrNilRedisClient
	}

	algorithm := rule.Algorithm
	if algorithm == "" {
		algorithm = TokenBucket
	}

	switch algorithm {
	case FixedWindow:
		return l.allowFixedWindow(ctx, rule)
	case SlidingWindow:
		return l.allowSlidingWindow(ctx, rule)
	case SlidingWindowCounter:
		return l.allowSlidingWindowCounter(ctx, rule)
	case TokenBucket:
		return l.allowTokenBucket(ctx, rule)
	default:
		return l.allowTokenBucket(ctx, rule)
	}
}

func (l *RedisLimiter) allowFixedWindow(ctx context.Context, rule Rule) (bool, error) {
	windowMillis := durationMillis(rule.Window)
	windowID := time.Now().UnixMilli() / windowMillis
	key := l.prefix + ":fixed:" + rule.Key + ":" + strconv.FormatInt(windowID, 10)

	timeoutCtx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	allowed, err := redisFixedWindowScript.Run(timeoutCtx, l.client, []string{key}, rule.Limit, windowMillis).Int()
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func (l *RedisLimiter) allowTokenBucket(ctx context.Context, rule Rule) (bool, error) {
	windowMillis := durationMillis(rule.Window)
	burst := rule.Burst
	if burst <= 0 {
		burst = rule.Limit
	}
	if burst <= 0 {
		return true, nil
	}

	nowMillis := time.Now().UnixMilli()
	refillRate := float64(rule.Limit) / float64(windowMillis)
	ttlMillis := windowMillis * 2
	if ttlMillis < int64(time.Second/time.Millisecond) {
		ttlMillis = int64(time.Second / time.Millisecond)
	}
	key := l.prefix + ":bucket:" + rule.Key

	timeoutCtx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	allowed, err := redisTokenBucketScript.Run(timeoutCtx, l.client, []string{key}, burst, refillRate, nowMillis, ttlMillis).Int()
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func (l *RedisLimiter) allowSlidingWindow(ctx context.Context, rule Rule) (bool, error) {
	windowMillis := durationMillis(rule.Window)
	nowMillis := time.Now().UnixMilli()
	ttlMillis := windowMillis * 2
	key := redisClusterKey(l.prefix, "sliding", rule.Key, "")
	seqKey := key + ":seq"

	timeoutCtx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	allowed, err := redisSlidingWindowScript.Run(timeoutCtx, l.client, []string{key, seqKey}, rule.Limit, windowMillis, nowMillis, ttlMillis).Int()
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func (l *RedisLimiter) allowSlidingWindowCounter(ctx context.Context, rule Rule) (bool, error) {
	windowMillis := durationMillis(rule.Window)
	nowMillis := time.Now().UnixMilli()
	windowID := nowMillis / windowMillis
	currentKey := redisClusterKey(l.prefix, "sliding_counter", rule.Key, strconv.FormatInt(windowID, 10))
	previousKey := redisClusterKey(l.prefix, "sliding_counter", rule.Key, strconv.FormatInt(windowID-1, 10))
	ttlMillis := windowMillis * 2

	timeoutCtx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	allowed, err := redisSlidingWindowCounterScript.Run(timeoutCtx, l.client, []string{currentKey, previousKey}, rule.Limit, windowMillis, nowMillis, ttlMillis).Int()
	if err != nil {
		return false, err
	}
	return allowed == 1, nil
}

func durationMillis(d time.Duration) int64 {
	millis := d.Milliseconds()
	if millis <= 0 {
		return int64(time.Second / time.Millisecond)
	}
	return millis
}

func redisClusterKey(prefix, namespace, key, suffix string) string {
	base := prefix + ":" + namespace + ":{" + key + "}"
	if suffix == "" {
		return base
	}
	return base + ":" + suffix
}

var redisFixedWindowScript = redis.NewScript(`
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
if current > tonumber(ARGV[1]) then
  return 0
end
return 1
`)

var redisTokenBucketScript = redis.NewScript(`
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local bucket = redis.call("HMGET", KEYS[1], "tokens", "timestamp")
local tokens = tonumber(bucket[1])
local timestamp = tonumber(bucket[2])

if tokens == nil then
  tokens = capacity
end
if timestamp == nil then
  timestamp = now
end

local delta = now - timestamp
if delta < 0 then
  delta = 0
end

tokens = math.min(capacity, tokens + delta * refill_rate)
local allowed = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
end

redis.call("HSET", KEYS[1], "tokens", tokens, "timestamp", now)
redis.call("PEXPIRE", KEYS[1], ttl)
return allowed
`)

var redisSlidingWindowScript = redis.NewScript(`
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])
local min_score = now - window

redis.call("ZREMRANGEBYSCORE", KEYS[1], 0, min_score)
local current = redis.call("ZCARD", KEYS[1])
if current >= limit then
  redis.call("PEXPIRE", KEYS[1], ttl)
  redis.call("PEXPIRE", KEYS[2], ttl)
  return 0
end

local seq = redis.call("INCR", KEYS[2])
redis.call("ZADD", KEYS[1], now, tostring(now) .. "-" .. tostring(seq))
redis.call("PEXPIRE", KEYS[1], ttl)
redis.call("PEXPIRE", KEYS[2], ttl)
return 1
`)

var redisSlidingWindowCounterScript = redis.NewScript(`
local limit = tonumber(ARGV[1])
local window = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local elapsed = now % window
local previous_weight = (window - elapsed) / window
local current = tonumber(redis.call("GET", KEYS[1]) or "0")
local previous = tonumber(redis.call("GET", KEYS[2]) or "0")
local estimated = current + previous * previous_weight

if estimated + 1 > limit then
  redis.call("PEXPIRE", KEYS[1], ttl)
  redis.call("PEXPIRE", KEYS[2], ttl)
  return 0
end

redis.call("INCR", KEYS[1])
redis.call("PEXPIRE", KEYS[1], ttl)
redis.call("PEXPIRE", KEYS[2], ttl)
return 1
`)
