#!/usr/bin/bash
set -euo pipefail
role=${1:-}
if [[ $role != parent && $role != kid ]]; then
  echo "usage: apply-role.sh parent|kid" >&2
  exit 2
fi
here=$(cd "$(dirname -- "$0")/.." && pwd)
if [[ ! -f $here/manifest.json || ! -f $here/packaging/config.kid.toml ]]; then
  echo "Kidtimer files are missing." >&2
  exit 1
fi

need_bin() {
  local dest=$1
  if [[ -x $dest ]]; then
    return 0
  fi
  if [[ -x $here/kidtimer ]]; then
    /usr/bin/install -m 0755 "$here/kidtimer" "$dest"
    return 0
  fi
  if command -v go >/dev/null 2>&1 && [[ -f $here/go.mod ]]; then
    /usr/bin/mkdir -p -- "$(dirname -- "$dest")"
    (cd "$here" && go build -o "$dest" ./daemon/cmd/kidtimer)
    return 0
  fi
  echo "Need the kidtimer binary. From this folder: go build -o kidtimer ./daemon/cmd/kidtimer" >&2
  exit 1
}

if [[ $role == parent ]]; then
  dest=${HOME}/.local/bin/kidtimer
  /usr/bin/mkdir -p -- "$(dirname -- "$dest")"
  need_bin "$dest"
  exec "$dest" setup parent -repo "$here"
fi

echo "Kidtimer needs your password to run the timer on this computer."
dest=/usr/local/bin/kidtimer
if [[ ! -x $dest ]]; then
  tmp=${HOME}/.local/bin/kidtimer
  /usr/bin/mkdir -p -- "$(dirname -- "$tmp")"
  need_bin "$tmp"
  exec /usr/bin/sudo "$tmp" setup kid -repo "$here"
fi
exec /usr/bin/sudo "$dest" setup kid -repo "$here"
