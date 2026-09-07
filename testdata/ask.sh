#!/bin/sh
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"

printf 'Authorization: Bearer %s\n' "$TOKEN" | curl -sS -X POST "$URL/v1/asks" \
  -H @- \
  -H "Content-Type: application/json" \
  -d '{"group":"fun","seconds":900,"reason":"one more video"}'
echo
