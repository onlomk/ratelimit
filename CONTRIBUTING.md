# Contributing

Thanks for your interest in contributing to `ratelimit`.

## Development setup

```bash
git clone git@github.com:onlomk/ratelimit.git
cd ratelimit
go mod download
```

## Run checks

```bash
go test ./...
go vet ./...
```

Run benchmarks:

```bash
go test -run "^$" -bench "." -benchmem ./...
```

Run Redis integration tests with a real Redis instance:

```bash
RATELIMIT_REDIS_ADDR=127.0.0.1:6379 go test ./...
```

The Redis tests use a unique key prefix and only clean up keys created by the tests.

## Pull request guidelines

- Keep changes focused and minimal.
- Add or update tests for behavior changes.
- Update README or examples when public usage changes.
- Avoid adding dependencies unless they are necessary.
- Do not include secrets, tokens, private URLs, or real user data in code, tests, or docs.

## Commit style

Use clear, concise commit messages, for example:

- `add memory limiter benchmark`
- `fix redis nil client handling`
- `document route-level middleware usage`
