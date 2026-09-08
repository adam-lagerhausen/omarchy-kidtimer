#!/usr/bin/bash
set -eu
URL="${KIDTIMER_URL:-http://127.0.0.1:8742}"
TOKEN="${KIDTIMER_TOKEN:?set KIDTIMER_TOKEN}"

printf 'Authorization: Bearer %s\n' "$TOKEN" | /usr/bin/curl -q -sS \
  --max-time 10 --connect-timeout 5 --max-filesize 1048576 --noproxy '*' \
  -H @- -- "$URL/v1/status"
echo
