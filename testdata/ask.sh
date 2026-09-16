#!/usr/bin/bash
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"

printf '%s' '{"group":"fun","seconds":900,"reason":"one more video"}' | /usr/bin/curl -q -sS -X POST \
  --max-time 10 --connect-timeout 5 --max-filesize 1048576 --noproxy '*' \
  -H @<(printf 'Authorization: Bearer %s\n' "$TOKEN") \
  -H "Content-Type: application/json" \
  --data-binary @- \
  -- "$URL/v1/asks"
echo
