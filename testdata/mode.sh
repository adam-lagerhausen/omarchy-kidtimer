#!/bin/sh
# Talks to a Stage B daemon on localhost. Does not enable the enforcer.
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"
MODE="${MODE:-morning}"

printf 'Authorization: Bearer %s\n' "$TOKEN" | curl -sS -X POST "$URL/v1/mode" \
  -H @- \
  -H "Content-Type: application/json" \
  -d "{\"id\":\"$MODE\"}"
echo
