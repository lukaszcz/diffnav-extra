#!/usr/bin/env bash
# check_coverage.sh validates that all testable packages have 100% test coverage.
# It checks per-package statement coverage (as reported by go test -cover)
# rather than per-function coverage to avoid false positives from empty
# functions that show 0.0% despite having no statements to cover.

set -euo pipefail

# Run tests with coverage for all testable packages.
# The root package (github.com/dlvhdr/diffnav) is excluded because it has no
# test files and triggers a covdata tool error under Go 1.25.
output=$(go test -count=1 -cover ./pkg/... ./cmd/... 2>&1)

fail=0

# Parse the output line by line, checking per-package coverage.
while IFS= read -r line; do
  # Skip lines without coverage info
  if ! echo "$line" | grep -q 'coverage:'; then
    continue
  fi

  # [no statements] packages are fine
  if echo "$line" | grep -q 'no statements'; then
    continue
  fi

  # Extract percentage (e.g. "100.0" from "coverage: 100.0% of statements")
  pct=$(echo "$line" | sed -n 's/.*coverage: *//p' | sed 's/%.*//')
  pkg=$(echo "$line" | awk '{print $2}')

  if [ -z "$pct" ]; then
    continue
  fi

  # Check if coverage is 100%
  if awk "BEGIN { exit ($pct < 100.0) }"; then
    : # 100% coverage
  else
    echo "FAIL: $pkg coverage is ${pct}% (need 100%)"
    fail=1
  fi
done <<< "$output"

# Print the raw output for visibility
echo "$output"

if [ "$fail" -eq 1 ]; then
  echo ""
  echo "Coverage check FAILED. Some packages do not have 100% coverage."
  exit 1
fi

echo ""
echo "✓ All packages have 100% test coverage."
exit 0
