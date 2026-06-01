# Examples

This directory contains runnable examples for common production usage patterns.

## Memory limiter

Basic in-process limiter usage:

```bash
go run ./examples/memory
```

## HTTP middleware

Route-level `net/http` middleware with default rules and per-route overrides:

```bash
go run ./examples/http_middleware
```

Then try:

```bash
curl http://127.0.0.1:8080/api/search
curl -H "X-User-ID: 123" http://127.0.0.1:8080/api/login
curl http://127.0.0.1:8080/health
```

## Redis fallback

Redis primary limiter with memory fallback:

```bash
go run ./examples/fallback
```

The fallback example can still run when Redis is unavailable because Redis errors are handled by falling back to the memory limiter.

## Gin middleware

Gin middleware with default rules, route-level overrides, and disabled health checks:

```bash
go run ./examples/gin
```

Then try:

```bash
curl http://127.0.0.1:8080/api/search
curl -H "X-User-ID: 123" http://127.0.0.1:8080/api/login
curl http://127.0.0.1:8080/health
```
