#!/bin/sh
# Runs each e2e suite against a FRESH server instance, so every suite starts
# from the seeded board and an empty SSE history.
#
#   e2e/run.sh            # all suites
#   e2e/run.sh replay     # just one
set -e
cd "$(dirname "$0")/.."

BIN="$(mktemp -d)/kanban"
go build -o "$BIN" .

SUITES="${*:-test replay}"
status=0
for suite in $SUITES; do
    echo "--- suite: $suite ---"
    "$BIN" >/dev/null 2>&1 &
    SERVER=$!
    sleep 1
    node "e2e/$suite.mjs" || status=1
    kill "$SERVER" 2>/dev/null || true
    wait "$SERVER" 2>/dev/null || true
done
exit $status
