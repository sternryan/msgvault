---
timeout: 180
primary_metric: coverage
direction: higher
scope: internal/search/*_test.go
metrics_cmd: ./metrics.sh
guards:
  pass_rate: 1.0
---

# Objective

Improve test coverage for the msgvault search package (`internal/search/`).

# Context

- The search package has `parser.go` with the main implementation
- Existing tests are in `parser_test.go` and `helpers_test.go`
- Build flag `-tags fts5` is required
- Current coverage can be checked with: `go test -tags fts5 ./internal/search/... -cover`

# Constraints

- You may ONLY modify or create `*_test.go` files in `internal/search/`
- Do NOT modify production code (parser.go, etc.)
- All tests must pass
- New tests should be meaningful — test real behavior, not trivial getters

# Ideas to Explore

- Edge cases in query parsing (empty queries, special characters, unicode)
- Error path coverage (malformed input, nil handling)
- Benchmark functions for performance regression detection
- Table-driven tests for parser edge cases
