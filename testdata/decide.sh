#!/bin/sh
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"
ID="${1:?ask id}"
DECISION="${2:-approve}"

printf 'Authorization: Bearer %s\n' "$TOKEN" | curl -sS -X POST "$URL/v1/asks/$ID/decide" \
  -H @- \
  -H "Content-Type: application/json" \
  -d "{\"decision\":\"$DECISION\"}"
echo
