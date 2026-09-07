#!/bin/sh
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"

printf 'Authorization: Bearer %s\n' "$TOKEN" | curl -sS -H @- "$URL/v1/status"
echo
