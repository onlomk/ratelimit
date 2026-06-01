package ginlimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/onlomk/ratelimit"
)

func TestMiddlewareRoutePolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := ratelimit.NewMemoryLimiter("test", time.Minute)
	defer limiter.Close()

	r := gin.New()
	r.Use(New(limiter, Route("/login", ratelimit.PerMinute(1).WithAlgorithm(ratelimit.FixedWindow))))
	r.GET("/login", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	perform(r, "/login")
	w := perform(r, "/login")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
}

func TestMiddlewareNoLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	limiter := ratelimit.NewMemoryLimiter("test", time.Minute)
	defer limiter.Close()

	r := gin.New()
	r.Use(New(limiter, Default(ratelimit.PerMinute(1)), NoLimit("/health")))
	r.GET("/health", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	perform(r, "/health")
	w := perform(r, "/health")
	if w.Code != http.StatusOK {
		t.Fatalf("expected no limit route to stay 200, got %d", w.Code)
	}
}

func perform(r http.Handler, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	return w
}
