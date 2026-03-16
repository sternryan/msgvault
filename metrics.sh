#!/usr/bin/env bash
set -euo pipefail

export PATH="$PATH:/opt/homebrew/bin"

# Run tests first (guard metric)
TEST_OUTPUT=$(go test -tags fts5 ./internal/search/... -v -count=1 2>&1) || {
    echo '{"pass_rate": 0.0, "bench_search_ns_per_op": 999999999, "coverage": 0, "test_count": 0}'
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

# Run benchmarks (if any exist)
BENCH_OUTPUT=$(go test -tags fts5 ./internal/search/... -bench=. -benchmem -count=3 -timeout=60s 2>&1) || true

# Extract benchmark ns/op (take median of 3 runs)
# Use a subshell to avoid pipefail killing us on empty grep
BENCH_NS=$(echo "$BENCH_OUTPUT" | { grep "Benchmark" || true; } | awk '{print $3}' | sort -n | head -2 | tail -1)
if [[ -z "$BENCH_NS" || "$BENCH_NS" == "" ]]; then
    BENCH_NS=0
fi

# Get test coverage
COVERAGE_OUTPUT=$(go test -tags fts5 ./internal/search/... -cover 2>&1) || true
COVERAGE=$(echo "$COVERAGE_OUTPUT" | { grep -o '[0-9]*\.[0-9]*%' || true; } | head -1 | tr -d '%')
if [[ -z "$COVERAGE" ]]; then
    COVERAGE=0
fi

# Output structured JSON
jq -n \
    --argjson pass_rate "$PASS_RATE" \
    --argjson bench_search_ns_per_op "${BENCH_NS}" \
    --argjson coverage "${COVERAGE}" \
    --argjson test_count "${TOTAL_TESTS}" \
    '{
        pass_rate: $pass_rate,
        bench_search_ns_per_op: $bench_search_ns_per_op,
        coverage: $coverage,
        test_count: $test_count
    }'
