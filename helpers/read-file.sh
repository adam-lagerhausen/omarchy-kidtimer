#!/usr/bin/bash
set -euo pipefail
file=${1:-}
if [[ -z $file || $file == *..* ]]; then
  echo "usage: read-file.sh PATH" >&2
  exit 2
fi
if [[ ! -e $file ]]; then
  exit 1
fi
# Open once; bash cannot O_NOFOLLOW a redirect. Refuse a symlink, then read a cap.
if [[ -L $file ]]; then
  echo "refusing symlink" >&2
  exit 1
fi
if [[ ! -f $file ]]; then
  echo "not a regular file" >&2
  exit 1
fi
exec /usr/bin/head -c 65537 -- "$file"
