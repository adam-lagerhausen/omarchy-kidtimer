#!/bin/sh
# Talks to a Stage B daemon on localhost. Does not enable the enforcer.
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"

printf 'Authorization: Bearer %s\n' "$TOKEN" | curl -sS -X PATCH "$URL/v1/policy" \
  -H @- \
  -H "Content-Type: application/json" \
  -d '{"bedtime_start":"20:00"}'
echo
