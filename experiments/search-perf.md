---
timeout: 900
model: opus
primary_metric: bench_parse_complex_ns
direction: lower
scope: internal/search/*.go
metrics_cmd: ./metrics.sh
guards:
  pass_rate: 1.0
  bench_parse_bare_ns: 130
---

# Objective

Improve FTS5 full-text search query parsing performance in the msgvault search package.
The primary target is BenchmarkParse_Complex — a realistic query with operators, dates,
sizes, quoted phrases, and labels. Currently ~1150 ns/op with 44 allocations.

# Context

- The search package is at `internal/search/`
- `parser.go` is the main implementation (~300 lines)
- The parser has three fast paths: empty query, bare words (no quotes/colons), and
  colon-but-no-quotes. The quoted-phrase path uses `tokenize()` with a strings.Builder.
- Previous optimizations already applied: package-level regex compilation, toLowerFast,
  switch-based dispatch, queryAlloc struct, classifyQuery pre-scan, IndexByte for colon search.
- The hot path for complex queries: tokenize → loop tokens → operator dispatch → type-specific parsing.
- Biggest allocation sources (44 allocs): queryAlloc (1), slice appends for each field,
  string conversions in toLowerFast, time.Parse in parseDate, strconv.ParseFloat in parseSize.

# Constraints

- Do NOT change the database schema
- Do NOT modify files outside `internal/search/`
- Do NOT add external dependencies
- All existing tests must continue to pass
- Do NOT regress BenchmarkParse_BareWords (guard: must stay under 130 ns/op)
- You may add new test helpers or benchmarks if they support your optimization

# Ideas to Explore

- Reduce allocations in tokenize() — the strings.Builder allocates on every quoted phrase
- Pool or pre-allocate the Query struct's slice fields (FromAddrs, ToAddrs, etc.)
- Avoid time.Parse overhead — cache parsed dates or use manual date parsing
- Reduce string copies in operator dispatch (substring slicing vs. allocation)
- Look at whether applyOperator can avoid string allocations for common operators
- Profile with `go test -cpuprofile` to find actual hot spots before optimizing
