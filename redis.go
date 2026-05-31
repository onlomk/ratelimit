package ratelimit

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisAlgorithm string

const (
	RedisFixedWindow RedisAlgorithm = "fixed_window"
	RedisTokenBucket RedisAlgorithm = "token_bucket"
)

const defaultRedisLimiterTimeout = 50 * time.Millisecond

const DefaultRedisLimiterPrefix = "rate_limit"

type RedisRule struct {
	Key       string
	Limit     int
	Burst     int
	Window    time.Duration
	Algorithm RedisAlgorithm
}

type RedisLimiter struct {
	client  *redis.Client
	prefix  string
	timeout time.Duration
}

func NewRedisLimiter(client *redis.Client, prefix string, timeout time.Duration) *RedisLimiter {
	if prefix == "" {
		prefix = DefaultRedisLimiterPrefix
	}
	if timeout <= 0 {
		timeout = defaultRedisLimiterTimeout
	}
	return &RedisLimiter{client: client, prefix: prefix, timeout: timeout}
}

func (l *RedisLimiter) Allow(ctx context.Context, rule RedisRule) (bool, error) {
	if rule.Limit <= 0 || rule.Window <= 0 {
		return true, nil
	}
	if rule.Key == "" {
		return true, nil
	}

	algorithm := rule.Algorithm
	if algorithm == "" {
		algorithm = RedisTokenBucket
	}

	switch algorithm {
	case RedisFixedWindow:
		return l.allowFixedWindow(ctx, rule)
	case RedisTokenBucket:
		return l.allowTokenBucket(ctx, rule)
	default:
		return l.allowTokenBucket(ctx, rule)
	}
}

func (l *RedisLimiter) allowFixedWindow(ctx context.Context, rule RedisRule) (bool, error) {
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

func (l *RedisLimiter) allowTokenBucket(ctx context.Context, rule RedisRule) (bool, error) {
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

func durationMillis(d time.Duration) int64 {
	millis := d.Milliseconds()
	if millis <= 0 {
		return int64(time.Second / time.Millisecond)
	}
	return millis
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
