#!/bin/sh
# Talks to a Stage B daemon on localhost. Does not enable the enforcer.
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"
LOCKED="${LOCKED:-true}"

curl -sS -X POST "$URL/v1/lock" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"locked\":$LOCKED}"
echo
