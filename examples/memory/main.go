package main

import (
	"context"
	"fmt"
	"time"

	"github.com/onlomk/ratelimit"
)

func main() {
	limiter := ratelimit.NewMemoryLimiter("example", 5*time.Minute)
	defer limiter.Close()

	rule := ratelimit.Rule{
		Key:       "user:123",
		Limit:     3,
		Burst:     3,
		Window:    time.Minute,
		Algorithm: ratelimit.TokenBucket,
	}

	for i := 1; i <= 5; i++ {
		allowed, err := limiter.Allow(context.Background(), rule)
		if err != nil {
			panic(err)
		}
		fmt.Printf("request %d allowed=%v\n", i, allowed)
	}
}
