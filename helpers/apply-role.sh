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

share=${HOME}/.local/share/kidtimer
/usr/bin/mkdir -p -- "$share"
rm -f -- "$share/setup-error"

fail() {
  echo "$1" >&2
  printf '%s\n' "$1" >"$share/setup-error"
  exit 1
}

goarch() {
  case $(uname -m) in
    x86_64 | amd64) echo amd64 ;;
    aarch64 | arm64) echo arm64 ;;
    *) echo "" ;;
  esac
}

elf_ok() {
  local bin=$1
  local want=$2
  [[ -x $bin ]] || return 1
  [[ -n $want ]] || return 0
  local bytes
  bytes=$(od -An -t x1 -j 18 -N 2 -- "$bin" 2>/dev/null | tr -d ' \n')
  case $want in
    amd64) [[ $bytes == 3e00 ]] ;;
    arm64) [[ $bytes == b700 ]] ;;
    *) return 0 ;;
  esac
}

need_bin() {
  local dest=$1
  local want
  want=$(goarch)
  /usr/bin/mkdir -p -- "$(dirname -- "$dest")"
  if [[ -n $want && -x $here/kidtimer-linux-$want ]] && elf_ok "$here/kidtimer-linux-$want" "$want"; then
    /usr/bin/install -m 0755 "$here/kidtimer-linux-$want" "$dest"
    return 0
  fi
  if elf_ok "$here/kidtimer" "$want"; then
    /usr/bin/install -m 0755 "$here/kidtimer" "$dest"
    return 0
  fi
  if elf_ok "$dest" "$want"; then
    return 0
  fi
  if command -v go >/dev/null 2>&1 && [[ -f $here/go.mod ]]; then
    if [[ -n $want ]]; then
      (cd "$here" && CGO_ENABLED=0 GOOS=linux GOARCH=$want go build -o "$dest" ./daemon/cmd/kidtimer)
    else
      (cd "$here" && CGO_ENABLED=0 go build -o "$dest" ./daemon/cmd/kidtimer)
    fi
    return 0
  fi
  fail "Need the kidtimer binary for this computer ($(uname -m))."
}

if [[ $role == parent ]]; then
  dest=${HOME}/.local/bin/kidtimer
  /usr/bin/mkdir -p -- "$(dirname -- "$dest")"
  need_bin "$dest"
  exec "$dest" setup parent -repo "$here"
fi

echo "Kidtimer needs your password to run the timer on this computer."
trap 'fail "Could not set up this computer."' ERR
dest=/usr/local/bin/kidtimer
if ! elf_ok "$dest" "$(goarch)"; then
  tmp=${HOME}/.local/bin/kidtimer
  /usr/bin/mkdir -p -- "$(dirname -- "$tmp")"
  need_bin "$tmp"
  /usr/bin/sudo "$tmp" setup kid -repo "$here"
  exit 0
fi
/usr/bin/sudo "$dest" setup kid -repo "$here"
