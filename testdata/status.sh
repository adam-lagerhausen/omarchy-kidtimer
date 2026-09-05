#!/bin/sh
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"

curl -sS "$URL/v1/status" \
  -H "Authorization: Bearer $TOKEN"
echo
