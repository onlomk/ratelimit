// Package ginlimit provides optional Gin middleware for ratelimit.
package ginlimit

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/onlomk/ratelimit"
)

var errNilLimiter = errors.New("ginlimit: nil limiter")

// KeyFunc builds a rate limit key for the current request.
type KeyFunc func(*gin.Context) string

// ErrorHandler handles limiter backend errors.
type ErrorHandler func(*gin.Context, error)

// RejectHandler handles rejected requests.
type RejectHandler func(*gin.Context)

type routeConfig struct {
	disabled bool
	policies []ratelimit.Policy
}

type config struct {
	defaultPolicies []ratelimit.Policy
	routes          map[string]routeConfig
	keyFunc         KeyFunc
	errorHandler    ErrorHandler
	rejectHandler   RejectHandler
}

// Option configures Gin middleware.
type Option func(*config)

// New creates Gin middleware with optional default and route-level policies.
func New(limiter ratelimit.Limiter, opts ...Option) gin.HandlerFunc {
	cfg := config{
		routes:        make(map[string]routeConfig),
		keyFunc:       defaultKey,
		errorHandler:  defaultErrorHandler,
		rejectHandler: defaultRejectHandler,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	return func(c *gin.Context) {
		if limiter == nil {
			cfg.errorHandler(c, errNilLimiter)
			return
		}

		path := routePath(c)
		policies := cfg.defaultPolicies
		if route, ok := cfg.routes[path]; ok {
			if route.disabled {
				c.Next()
				return
			}
			policies = route.policies
		}
		if len(policies) == 0 {
			c.Next()
			return
		}

		allowed, err := ratelimit.AllowAll(c.Request.Context(), limiter, path+":"+cfg.keyFunc(c), policies...)
		if err != nil {
			cfg.errorHandler(c, err)
			return
		}
		if !allowed {
			cfg.rejectHandler(c)
			return
		}

		c.Next()
	}
}

// Default sets policies used by routes without an override.
func Default(policies ...ratelimit.Policy) Option {
	return func(cfg *config) {
		cfg.defaultPolicies = policies
	}
}

// Route overrides policies for path.
func Route(path string, policies ...ratelimit.Policy) Option {
	return func(cfg *config) {
		cfg.routes[path] = routeConfig{policies: policies}
	}
}

// NoLimit disables limiting for path.
func NoLimit(path string) Option {
	return func(cfg *config) {
		cfg.routes[path] = routeConfig{disabled: true}
	}
}

// WithKeyFunc sets the key function. The route path is still included by the
// middleware to avoid sharing one quota across unrelated routes.
func WithKeyFunc(fn KeyFunc) Option {
	return func(cfg *config) {
		if fn != nil {
			cfg.keyFunc = fn
		}
	}
}

// WithErrorHandler sets the backend error handler.
func WithErrorHandler(fn ErrorHandler) Option {
	return func(cfg *config) {
		if fn != nil {
			cfg.errorHandler = fn
		}
	}
}

// WithRejectHandler sets the rejected request handler.
func WithRejectHandler(fn RejectHandler) Option {
	return func(cfg *config) {
		if fn != nil {
			cfg.rejectHandler = fn
		}
	}
}

func defaultKey(c *gin.Context) string {
	// X-User-ID must come from a trusted authentication layer or upstream proxy.
	// Do not trust this header when clients can set it directly.
	if userID := c.GetHeader("X-User-ID"); userID != "" {
		return "user:" + userID
	}
	return "ip:" + c.ClientIP()
}

func routePath(c *gin.Context) string {
	if path := c.FullPath(); path != "" {
		return path
	}
	return c.Request.URL.Path
}

func defaultErrorHandler(c *gin.Context, _ error) {
	c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "rate limit unavailable"})
}

func defaultRejectHandler(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "too many requests"})
}
