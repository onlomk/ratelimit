package main

import (
	"log"
	"net/http"
	"time"

	"github.com/onlomk/ratelimit"
)

type RouteRule struct {
	Limit     int
	Burst     int
	Window    time.Duration
	Algorithm ratelimit.Algorithm
	Disabled  bool
}

func main() {
	limiter := ratelimit.NewMemoryLimiter("api", 5*time.Minute)
	defer limiter.Close()

	defaultRule := RouteRule{
		Limit:     100,
		Burst:     100,
		Window:    time.Minute,
		Algorithm: ratelimit.TokenBucket,
	}
	routeRules := map[string]RouteRule{
		"/api/login":  {Limit: 5, Burst: 5, Window: time.Minute, Algorithm: ratelimit.FixedWindow},
		"/api/search": {Limit: 10, Burst: 10, Window: time.Second, Algorithm: ratelimit.TokenBucket},
		"/api/export": {Limit: 3, Burst: 3, Window: time.Hour, Algorithm: ratelimit.SlidingWindowCounter},
		"/health":     {Disabled: true},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", ok)
	mux.HandleFunc("/api/search", ok)
	mux.HandleFunc("/api/export", ok)
	mux.HandleFunc("/health", ok)

	handler := RateLimitMiddleware(limiter, defaultRule, routeRules, mux)
	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

func ok(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
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
