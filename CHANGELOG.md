# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows semantic versioning where possible.

## [Unreleased]

### Added

- GitHub Actions CI for tests and `go vet`.
- Runnable examples for memory limiting, HTTP middleware, and Redis fallback.
- Chinese README and language switch links.
- Benchmarks for memory limiter hot paths.
- Package-level Go documentation.

## [0.1.0] - 2026-06-01

### Added

- Memory limiter backend.
- Redis limiter backend using Lua scripts.
- Unified `Limiter` interface and `Rule` type.
- Token bucket, fixed window, sliding window, and sliding window counter algorithms.
- Redis integration tests guarded by `RATELIMIT_REDIS_ADDR`.
- MIT License.
