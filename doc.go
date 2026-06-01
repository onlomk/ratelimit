// Package ratelimit provides production-ready rate limiting for Go services.
//
// It exposes a small Limiter interface and supports both in-process memory
// limiting and Redis-backed distributed limiting. The package is designed for
// HTTP middleware, API gateways, login protection, tenant quotas, SaaS plan
// enforcement, and backend service protection.
//
// The default algorithm is TokenBucket. Empty keys, non-positive limits, or
// non-positive windows are treated as unlimited to make optional rules easy to
// compose in route-level middleware.
package ratelimit
