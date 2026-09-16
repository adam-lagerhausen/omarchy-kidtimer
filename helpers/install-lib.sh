#!/usr/bin/bash
# Shared kid privileged-copy. Sourced by apply-role.sh and packaging/install.sh.

kidtimer_plugin_relpaths() {
  local src=$1
  local f d
  (
    cd "$src" || exit 1
    for f in manifest.json BarWidget.qml Panel.qml Overlay.qml Setup.qml ParentPanel.qml KidPanel.qml ParentModel.js KidModel.js Tape.qml LookBtn.qml; do
      if [[ -f $f ]]; then
        printf '%s\n' "$f"
      fi
    done
    for d in packaging helpers fonts icons; do
      if [[ -d $d ]]; then
        /usr/bin/find "$d" -type f ! -name '*.pyc' ! -path '*/__pycache__/*' -print
      fi
    done
  ) | /usr/bin/sort
}

kidtimer_manifest() {
  local src=$1
  local bin=$2
  /usr/bin/sha256sum -- "$bin" | /usr/bin/awk '{print $1 "  kidtimer"}'
  local rel
  while IFS= read -r rel; do
    [[ -n $rel ]] || continue
    /usr/bin/sha256sum -- "$src/$rel" | /usr/bin/awk -v p="$rel" '{print $1 "  " p}'
  done < <(kidtimer_plugin_relpaths "$src")
}

kidtimer_run_privileged_install() {
  local src=$1
  local bin=$2
  local here_helpers installer manifest
  here_helpers=$(cd "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
  installer=$(/usr/bin/cat -- "$here_helpers/privileged-install.py")
  [[ -n $installer ]] || return 1
  manifest=$(kidtimer_manifest "$src" "$bin")
  [[ -n $manifest ]] || return 1
  # Bytes are already in this process. Root must not reopen the user-owned helper path.
  printf '%s\n' "$installer" | /usr/bin/sudo /usr/bin/env \
    KIDTIMER_INSTALL_MANIFEST="$manifest" \
    /usr/bin/python3 -I -S - \
    --src "$src" --bin "$bin" \
    --dest-bin /usr/local/bin/kidtimer \
    --dest-share /usr/local/share/kidtimer \
    --run-setup
}
