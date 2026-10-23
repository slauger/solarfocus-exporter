#!/usr/bin/env bash
# Usage: hack/check-coverage.sh cover.out <min-percent>
set -euo pipefail
profile=${1:?coverage profile}
min=${2:?minimum percent}
total=$(go tool cover -func="$profile" | awk '/^total:/ {sub("%","",$3); print $3}')
echo "total coverage: ${total}% (minimum ${min}%)"
awk -v t="$total" -v m="$min" 'BEGIN { exit (t+0 >= m+0) ? 0 : 1 }'
