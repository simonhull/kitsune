#!/bin/sh
# Measure how fast Kitsune responds, against perf/budgets.json: the server's
# speed tests, then the browser suite (web/perf) on a release build serving
# generated libraries of 1,000 and 50,000 songs. Everything it writes goes to
# $KITSUNE_PERF_DIR (default: kitsune-perf in the temp directory); the
# libraries stay there for the next run. Arguments go to Playwright.
set -eu
cd "$(dirname "$0")/.."
KITSUNE_PERF_DIR=${KITSUNE_PERF_DIR:-${TMPDIR:-/tmp}/kitsune-perf}
export KITSUNE_PERF_DIR
mkdir -p "$KITSUNE_PERF_DIR"
go test -count=1 -run 'Speed$' ./internal/server
sh scripts/build.sh "$KITSUNE_PERF_DIR/kitsune-server"
cd web
bunx playwright test -c perf/playwright.config.ts "$@"
