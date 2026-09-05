#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

BENCH_DIR="${ROOT_DIR}/.benchmarks"
mkdir -p "$BENCH_DIR"
BASELINE_FILE="${BENCH_DIR}/baseline.txt"
CURRENT_FILE="${BENCH_DIR}/current.txt"

RECORD_MODE=0
COMPARE_MODE=0
COMPARE_FILE1=""
COMPARE_FILE2=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        --record)
            RECORD_MODE=1
            shift
            ;;
        --compare)
            COMPARE_MODE=1
            if [[ $# -ge 3 ]]; then
                COMPARE_FILE1="$2"
                COMPARE_FILE2="$3"
                shift 3
            else
                shift
            fi
            ;;
        --help|-h)
            echo "Usage: ./scripts/benchmark.sh [options]"
            echo ""
            echo "Options:"
            echo "  (no args)           Run standard benchmarks and display results"
            echo "  --record            Run benchmarks and save output to .benchmarks/baseline.txt"
            echo "  --compare [F1 F2]   Compare benchmark results (default: baseline.txt vs current.txt)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

BENCH_PACKAGES=(
    "./internal/database"
    "./internal/architecture"
    "./internal/core"
    "./internal/core/battle"
    "./internal/player"
)

if [[ $COMPARE_MODE -eq 1 ]]; then
    F1="${COMPARE_FILE1:-$BASELINE_FILE}"
    F2="${COMPARE_FILE2:-$CURRENT_FILE}"
    if [[ ! -f "$F1" ]]; then
        echo "Error: Baseline benchmark file not found: $F1"
        echo "Run './scripts/benchmark.sh --record' first to establish a baseline."
        exit 1
    fi
    if [[ ! -f "$F2" ]]; then
        echo "Error: Target benchmark file not found: $F2"
        exit 1
    fi
    echo "==> Comparing benchmarks: $F1 vs $F2"
    go run ./scripts/benchcompare "$F1" "$F2"
    exit 0
fi

echo "==> Running Continuous Performance Benchmarks..."
TEMP_OUTPUT=$(mktemp)
trap 'rm -f "$TEMP_OUTPUT"' EXIT

PARTY2_VALKEY_ADDR="${PARTY2_VALKEY_ADDR:-127.0.0.1:6379}" \
go test -run=^$ -bench=. -benchmem "${BENCH_PACKAGES[@]}" | tee "$TEMP_OUTPUT"

cp "$TEMP_OUTPUT" "$CURRENT_FILE"
echo "==> Current benchmark results saved to: $CURRENT_FILE"

if [[ $RECORD_MODE -eq 1 ]]; then
    cp "$TEMP_OUTPUT" "$BASELINE_FILE"
    echo "==> Baseline benchmark recorded at: $BASELINE_FILE"
elif [[ -f "$BASELINE_FILE" ]]; then
    echo ""
    echo "==> Evaluating against baseline ($BASELINE_FILE)..."
    go run ./scripts/benchcompare "$BASELINE_FILE" "$CURRENT_FILE" || true
fi

echo "==> Benchmark run complete!"
