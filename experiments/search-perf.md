---
timeout: 180
primary_metric: bench_search_ns_per_op
direction: lower
scope: internal/search/*.go
metrics_cmd: ./metrics.sh
guards:
  pass_rate: 1.0
---

# Objective

Improve FTS5 full-text search query performance in the msgvault search package.
The search index contains 452K email messages.

# Context

- The search package is at `internal/search/`
- It contains `parser.go` (main implementation), `parser_test.go`, and `helpers_test.go`
- msgvault uses SQLite FTS5 for full-text search via `go-sqlcipher`
- Build flag `-tags fts5` is required for all Go commands

# Constraints

- Do NOT change the database schema
- Do NOT modify files outside `internal/search/`
- Do NOT add external dependencies
- All existing tests must continue to pass
- You may add new test helpers or benchmarks if they support your optimization

# Ideas to Explore

- Query plan optimization (EXPLAIN QUERY PLAN)
- FTS5 content= syntax for prefix queries
- Search result pagination efficiency
- Query parsing optimizations in parser.go
- Index hint optimization
