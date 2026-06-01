package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/onlomk/ratelimit"
	"github.com/onlomk/ratelimit/middleware/ginlimit"
)

func main() {
	limiter := ratelimit.NewMemoryLimiter("gin", 5*time.Minute)
	defer limiter.Close()

	r := gin.Default()
	r.Use(ginlimit.New(limiter,
		ginlimit.Default(ratelimit.PerMinute(100)),
		ginlimit.Route("/api/login", ratelimit.PerMinute(5).WithAlgorithm(ratelimit.FixedWindow)),
		ginlimit.Route("/api/search", ratelimit.PerSecond(10)),
		ginlimit.Route("/api/export", ratelimit.PerHour(3).WithAlgorithm(ratelimit.SlidingWindowCounter)),
		ginlimit.NoLimit("/health"),
	))

	r.GET("/api/login", ok)
	r.GET("/api/search", ok)
	r.GET("/api/export", ok)
	r.GET("/health", ok)

	_ = r.Run(":8080")
}

func ok(c *gin.Context) {
	c.String(http.StatusOK, "ok\n")
}
