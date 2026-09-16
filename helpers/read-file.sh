#!/usr/bin/bash
set -euo pipefail
# Back-compat wrapper. Prefer helpers/state.py from QML.
name=${1:-}
if [[ $name == */* ]]; then
  name=${name##*/}
fi
exec /usr/bin/python3 -I -S "$(cd "$(dirname -- "$0")" && pwd)/state.py" read "$name"
