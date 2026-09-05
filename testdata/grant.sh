#!/bin/sh
# Talks to a Stage B daemon on localhost. Does not enable the enforcer.
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"
KEY="${IDEMPOTENCY_KEY:-testdata-grant-1}"

curl -sS -X POST "$URL/v1/grants" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Idempotency-Key: $KEY" \
  -H "Content-Type: application/json" \
  -d '{"group":"fun","seconds":900,"reason":"good afternoon"}'
echo
