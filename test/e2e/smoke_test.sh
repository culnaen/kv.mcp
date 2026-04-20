#!/usr/bin/env bash
set -euo pipefail

BINARY=/tmp/kv.mcp.test
DB=/tmp/self-smoke.db
PASS=0

fail() {
  printf "FAIL: %s\n" "$1"
  exit 1
}

# Step 1: build
printf "Building binary...\n"
CGO_ENABLED=0 go build -o "$BINARY" . || fail "build failed"

# Step 2: index
printf "Indexing project...\n"
"$BINARY" index --db "$DB" . || fail "index failed"

# Step 3: search for "Store"
printf "Searching for 'Store'...\n"
RESPONSE=$(printf '%s\n%s\n%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search","arguments":{"query":"Store"}}}' \
  '{"jsonrpc":"2.0","id":3,"method":"shutdown","params":{}}' \
  | "$BINARY" serve --db "$DB" --root .)

printf "%s\n" "$RESPONSE" | grep -q '"result"' || fail "search 'Store' response missing 'result'"

# Step 4: search for "ScanMerged"
printf "Searching for 'ScanMerged'...\n"
RESPONSE=$(printf '%s\n%s\n%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"0.1"}}}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search","arguments":{"query":"ScanMerged"}}}' \
  '{"jsonrpc":"2.0","id":3,"method":"shutdown","params":{}}' \
  | "$BINARY" serve --db "$DB" --root .)

printf "%s\n" "$RESPONSE" | grep -q '"result"' || fail "search 'ScanMerged' response missing 'result'"

printf "PASS\n"
exit 0
