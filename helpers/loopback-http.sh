#!/usr/bin/bash
set -euo pipefail
method=${1:-GET}
url=${2:-}
idem=${3:-}
max=1048576
case $url in
  http://127.0.0.1:8741/* | http://127.0.0.1:8742/*) ;;
  *)
    echo "refusing non-loopback" >&2
    exit 2
    ;;
esac
case $method in
  GET | POST | PUT | PATCH) ;;
  *)
    echo "refusing method" >&2
    exit 2
    ;;
esac
IFS= read -r token || true
body=$(/usr/bin/cat)
case $token in
  *$'\r'* | *$'\n'*)
    echo "refusing token" >&2
    exit 2
    ;;
esac
curl=(
  /usr/bin/curl -q -sS
  --max-time 10
  --connect-timeout 5
  --max-filesize "$max"
  --noproxy '*'
  --proto '=http'
  -X "$method"
)
if [[ -n $idem ]]; then
  curl+=(-H "Idempotency-Key: $idem")
fi
if [[ $method != GET ]]; then
  curl+=(-H "Content-Type: application/json" --data-binary @-)
fi
if [[ $method != GET ]]; then
  if [[ -n $token ]]; then
    printf '%s' "$body" | "${curl[@]}" -w '\n%{http_code}' -H @<(printf 'Authorization: Bearer %s\n' "$token") -- "$url"
  else
    printf '%s' "$body" | "${curl[@]}" -w '\n%{http_code}' -- "$url"
  fi
elif [[ -n $token ]]; then
  printf 'Authorization: Bearer %s\n' "$token" | "${curl[@]}" -w '\n%{http_code}' -H @- -- "$url"
else
  "${curl[@]}" -w '\n%{http_code}' -- "$url"
fi | /usr/bin/head -c $((max + 1))
