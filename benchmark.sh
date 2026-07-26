#!/bin/bash

set -e -o pipefail

mkdir -p benchmarks

GOOS=$(go env GOOS)
GOARCH=$(go env GOARCH)

# Extract CPU model safely across platforms
if [ "$GOOS" = "darwin" ] && command -v sysctl >/dev/null 2>&1; then
    CPU_RAW=$(sysctl -n machdep.cpu.brand_string)
elif [ -f /proc/cpuinfo ]; then
    CPU_RAW=$(awk -F: '/model name/ {print $2; exit}' /proc/cpuinfo)
elif command -v wmic >/dev/null 2>&1; then
    CPU_RAW=$(wmic cpu get name | sed -n '2p' | tr -d '\r')
else
    CPU_RAW="unknown"
fi

# Format: lowercase, strip "apple ", replace non-alphanumeric with underscores, trim leading/trailing underscores
CPU=$(echo "$CPU_RAW" | tr '[:upper:]' '[:lower:]' | sed -E 's/apple //g; s/[^a-z0-9]+/_/g; s/^_|_$//g')
OUT="benchmarks/${GOOS}-${GOARCH}-${CPU}"

echo "Running microbenchmarks..."
echo "Results will be saved to ${OUT}"

if [ -n "$1" ]; then
    go test -benchmem -run='^$' -bench=. ./... | tee "${OUT}" "$1"
else
    go test -benchmem -run='^$' -bench=. ./... | tee "${OUT}"
fi

