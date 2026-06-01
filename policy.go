package ratelimit

import (
	"context"
	"errors"
	"strconv"
	"time"
)

var ErrNilLimiter = errors.New("ratelimit: nil limiter")

// Policy is a reusable rate limit rule template.
//
// It is useful for route-level middleware where the key is known at request
// time but the limit configuration is static.
type Policy struct {
	Limit     int
	Burst     int
	Window    time.Duration
	Algorithm Algorithm
}

// Per creates a policy for a custom time window.
func Per(window time.Duration, limit int) Policy {
	return Policy{Limit: limit, Burst: limit, Window: window, Algorithm: TokenBucket}
}

// PerSecond creates a per-second policy.
func PerSecond(limit int) Policy {
	return Per(time.Second, limit)
}

// PerMinute creates a per-minute policy.
func PerMinute(limit int) Policy {
	return Per(time.Minute, limit)
}

// PerHour creates a per-hour policy.
func PerHour(limit int) Policy {
	return Per(time.Hour, limit)
}

// WithBurst returns a copy of the policy with a custom burst size.
func (p Policy) WithBurst(burst int) Policy {
	p.Burst = burst
	return p
}

// WithAlgorithm returns a copy of the policy with a custom algorithm.
func (p Policy) WithAlgorithm(algorithm Algorithm) Policy {
	p.Algorithm = algorithm
	return p
}

// Rule builds a concrete Rule for key.
func (p Policy) Rule(key string) Rule {
	burst := p.Burst
	if burst <= 0 {
		burst = p.Limit
	}
	return Rule{Key: key, Limit: p.Limit, Burst: burst, Window: p.Window, Algorithm: p.Algorithm}
}

// AllowAll checks all policies for key and rejects when any policy rejects.
func AllowAll(ctx context.Context, limiter Limiter, key string, policies ...Policy) (bool, error) {
	if len(policies) == 0 {
		return true, nil
	}
	if limiter == nil {
		return false, ErrNilLimiter
	}
	for _, policy := range policies {
		allowed, err := limiter.Allow(ctx, policy.Rule(policyKey(key, policy)))
		if err != nil {
			return false, err
		}
		if !allowed {
			return false, nil
		}
	}
	return true, nil
}

func policyKey(key string, policy Policy) string {
	return key + ":" + string(policy.Algorithm) + ":" + policy.Window.String() + ":" + strconv.Itoa(policy.Limit)
}
