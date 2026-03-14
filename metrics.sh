#!/usr/bin/env bash
set -euo pipefail

export PATH="$PATH:/opt/homebrew/bin"

# Run tests first (guard metric)
TEST_OUTPUT=$(go test -tags fts5 ./internal/search/... -v -count=1 2>&1) || {
    echo '{"pass_rate": 0.0, "bench_parse_complex_ns": 999999999, "bench_parse_bare_ns": 999999999, "coverage": 0, "test_count": 0}'
    exit 0
}

# Count pass/fail
TOTAL_TESTS=$(echo "$TEST_OUTPUT" | grep -c "^--- " || echo "0")
PASS_TESTS=$(echo "$TEST_OUTPUT" | grep -c "^--- PASS" || echo "0")
if [[ "$TOTAL_TESTS" -gt 0 ]]; then
    PASS_RATE=$(jq -n --argjson p "$PASS_TESTS" --argjson t "$TOTAL_TESTS" '($p / $t * 100 | round) / 100')
else
    if echo "$TEST_OUTPUT" | grep -q "^ok"; then
        PASS_RATE="1.0"
        TOTAL_TESTS=$(echo "$TEST_OUTPUT" | grep -c "=== RUN" || echo "0")
        PASS_TESTS="$TOTAL_TESTS"
    else
        PASS_RATE="0.0"
    fi
fi

# Run benchmarks — target specific benchmarks, not global min
BENCH_OUTPUT=$(go test -tags fts5 ./internal/search/... -bench=. -benchmem -count=3 -timeout=60s 2>&1) || true

# Extract ns/op for specific benchmarks (median of 3 runs = sort + take middle)
# BenchmarkParse_Complex: realistic workload (operators, dates, sizes, quotes, labels)
BENCH_COMPLEX=$(echo "$BENCH_OUTPUT" | { grep "BenchmarkParse_Complex" || true; } | awk '{print $3}' | sort -n | head -2 | tail -1)
[[ -z "$BENCH_COMPLEX" ]] && BENCH_COMPLEX=0

# BenchmarkParse_BareWords: common case (simple text search)
BENCH_BARE=$(echo "$BENCH_OUTPUT" | { grep "BenchmarkParse_BareWords" || true; } | awk '{print $3}' | sort -n | head -2 | tail -1)
[[ -z "$BENCH_BARE" ]] && BENCH_BARE=0

# BenchmarkParse_MultipleOperators: operator-heavy queries
BENCH_OPS=$(echo "$BENCH_OUTPUT" | { grep "BenchmarkParse_MultipleOperators" || true; } | awk '{print $3}' | sort -n | head -2 | tail -1)
[[ -z "$BENCH_OPS" ]] && BENCH_OPS=0

# Get test coverage
COVERAGE_OUTPUT=$(go test -tags fts5 ./internal/search/... -cover 2>&1) || true
COVERAGE=$(echo "$COVERAGE_OUTPUT" | { grep -o '[0-9]*\.[0-9]*%' || true; } | head -1 | tr -d '%')
if [[ -z "$COVERAGE" ]]; then
    COVERAGE=0
fi

# Output structured JSON — one metric per benchmark, not a global min
jq -n \
    --argjson pass_rate "$PASS_RATE" \
    --argjson bench_parse_complex_ns "$BENCH_COMPLEX" \
    --argjson bench_parse_bare_ns "$BENCH_BARE" \
    --argjson bench_parse_operators_ns "$BENCH_OPS" \
    --argjson coverage "${COVERAGE}" \
    --argjson test_count "${TOTAL_TESTS}" \
    '{
        pass_rate: $pass_rate,
        bench_parse_complex_ns: $bench_parse_complex_ns,
        bench_parse_bare_ns: $bench_parse_bare_ns,
        bench_parse_operators_ns: $bench_parse_operators_ns,
        coverage: $coverage,
        test_count: $test_count
    }'
