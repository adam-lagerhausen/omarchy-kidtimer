#!/bin/sh
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"

printf 'Authorization: Bearer %s\n' "$TOKEN" | curl -sS -X POST "$URL/v1/tokens" \
  -H @- \
  -H "Content-Type: application/json" \
  -d '{"name":"khan-webhook","kind":"app","max_seconds_per_grant":600,"max_seconds_per_day":1800}'
echo
