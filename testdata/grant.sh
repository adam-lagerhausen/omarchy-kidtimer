#!/usr/bin/bash
# Talks to a Stage B daemon on localhost. Does not enable the enforcer.
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"
KEY="${IDEMPOTENCY_KEY:-testdata-grant-1}"

printf '%s' '{"group":"fun","seconds":900,"reason":"good afternoon"}' | /usr/bin/curl -q -sS -X POST \
  --max-time 10 --connect-timeout 5 --max-filesize 1048576 --noproxy '*' \
  -H @<(printf 'Authorization: Bearer %s\n' "$TOKEN") \
  -H "Idempotency-Key: $KEY" \
  -H "Content-Type: application/json" \
  --data-binary @- \
  -- "$URL/v1/grants"
echo
