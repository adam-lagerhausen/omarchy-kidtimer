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
/usr/bin/chmod 700 -- "$share" 2>/dev/null || true

fail() {
  echo "$1" >&2
  umask 077
  printf '%s\n' "$1" >"$share/setup-error"
  /usr/bin/chmod 600 -- "$share/setup-error" 2>/dev/null || true
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

plugin_version() {
  /usr/bin/python3 -I -S -c 'import json,sys; print(json.load(open(sys.argv[1], encoding="utf-8"))["version"])' "$here/manifest.json"
}

sum_for() {
  local name=$1
  local line
  line=$(/usr/bin/awk -v n="$name" '$2==n {print $1; exit}' "$here/packaging/SHA256SUMS")
  [[ -n $line ]] || return 1
  printf '%s\n' "$line"
}

download_pinned() {
  local dest=$1
  local want=$2
  local ver name url sums got tmp extract bin tree
  [[ -n $want ]] || return 1
  [[ -f $here/packaging/SHA256SUMS ]] || return 1
  ver=$(plugin_version) || return 1
  name=kidtimer-linux-$want
  sums=$(sum_for "$name.tar.gz") || return 1
  url=https://github.com/adam-lagerhausen/omarchy-kidtimer/releases/download/v${ver}/${name}.tar.gz
  tmp=$(/usr/bin/mktemp -d)
  if ! /usr/bin/curl -q --fail --proto '=https' --proto-redir '=https' \
    --max-filesize 52428800 --max-time 60 --connect-timeout 15 \
    --noproxy '*' -o "$tmp/$name.tar.gz" -- "$url"; then
    /usr/bin/rm -rf -- "$tmp"
    return 1
  fi
  got=$(/usr/bin/sha256sum -- "$tmp/$name.tar.gz" | /usr/bin/awk '{print $1}')
  if [[ $got != "$sums" ]]; then
    echo "checksum mismatch for $name.tar.gz" >&2
    /usr/bin/rm -rf -- "$tmp"
    return 1
  fi
  extract=$tmp/out
  /usr/bin/mkdir -p -- "$extract"
  if ! /usr/bin/tar -C "$extract" -xzf "$tmp/$name.tar.gz" --no-same-owner; then
    /usr/bin/rm -rf -- "$tmp"
    return 1
  fi
  bin=$extract/$name/$name
  if ! elf_ok "$bin" "$want"; then
    bin=$(/usr/bin/find "$extract" -type f -name "$name" -print -quit)
  fi
  if ! elf_ok "$bin" "$want"; then
    /usr/bin/rm -rf -- "$tmp"
    return 1
  fi
  tree=$extract/$name
  if [[ ! -f $tree/manifest.json ]]; then
    tree=$extract
  fi
  /usr/bin/mkdir -p -- "$(dirname -- "$dest")"
  /usr/bin/install -m 0755 "$bin" "$dest"
  PINNED_TREE=$share/pinned-unpack
  /usr/bin/rm -rf -- "$PINNED_TREE"
  copy_plugin_tree "$tree" "$PINNED_TREE"
  /usr/bin/rm -rf -- "$tmp"
  return 0
}

copy_plugin_tree() {
  local src=$1
  local dest=$2
  /usr/bin/mkdir -p -- "$dest"
  local f
  for f in manifest.json BarWidget.qml Panel.qml Overlay.qml Setup.qml ParentPanel.qml KidPanel.qml ParentModel.js KidModel.js Tape.qml LookBtn.qml; do
    if [[ -e $src/$f ]]; then
      /usr/bin/cp -a -- "$src/$f" "$dest/"
    fi
  done
  for f in packaging helpers fonts icons; do
    if [[ -e $src/$f ]]; then
      /usr/bin/cp -a -- "$src/$f" "$dest/"
    fi
  done
}

need_bin() {
  local dest=$1
  local want
  want=$(goarch)
  /usr/bin/mkdir -p -- "$(dirname -- "$dest")"
  PINNED_TREE=
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
  if download_pinned "$dest" "$want"; then
    return 0
  fi
  if command -v go >/dev/null 2>&1 && [[ -f $here/go.mod ]]; then
    if [[ -n $want ]]; then
      (cd "$here" && CGO_ENABLED=0 GOOS=linux GOARCH=$want go build -trimpath -o "$dest" ./daemon/cmd/kidtimer)
    else
      (cd "$here" && CGO_ENABLED=0 go build -trimpath -o "$dest" ./daemon/cmd/kidtimer)
    fi
    return 0
  fi
  fail "Need the kidtimer binary for this computer ($(uname -m))."
}

if [[ $role == parent ]]; then
  current=
  if [[ -f $share/role ]]; then
    current=$(/usr/bin/tr -d '[:space:]' <"$share/role" 2>/dev/null || true)
  fi
  if [[ $current == kid ]] || [[ -f /etc/kidtimer/config.toml ]] || [[ -f /etc/systemd/system/kidtimer.service ]]; then
    fail "This computer is already a kid. Uninstall Kidtimer to use it as the parent desk."
  fi
  dest=${HOME}/.local/bin/kidtimer
  /usr/bin/mkdir -p -- "$(dirname -- "$dest")"
  need_bin "$dest"
  exec "$dest" setup parent -repo "$here"
fi

echo "Kidtimer needs your password to run the timer on this computer."
trap 'fail "Could not set up this computer."' ERR
userbin=${HOME}/.local/bin/kidtimer
need_bin "$userbin"
"$userbin" setup stop-user-bank || true

stage=$(/usr/bin/mktemp -d)
src=$here
if [[ -n ${PINNED_TREE:-} && -f $PINNED_TREE/packaging/config.kid.toml ]]; then
  src=$PINNED_TREE
fi
copy_plugin_tree "$src" "$stage"
/usr/bin/sudo /usr/bin/install -o root -g root -m 0755 -- "$userbin" /usr/local/bin/kidtimer
/usr/bin/sudo /usr/bin/rm -rf -- /usr/local/share/kidtimer
/usr/bin/sudo /usr/bin/mkdir -p -- /usr/local/share/kidtimer
/usr/bin/sudo /usr/bin/cp -a -- "$stage"/. /usr/local/share/kidtimer/
/usr/bin/sudo /usr/bin/chown -R root:root -- /usr/local/share/kidtimer
/usr/bin/rm -rf -- "$stage" "${PINNED_TREE:-}"
/usr/bin/sudo /usr/local/bin/kidtimer setup kid -repo /usr/local/share/kidtimer
