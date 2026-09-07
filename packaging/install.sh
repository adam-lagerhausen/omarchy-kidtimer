#!/bin/bash
set -euo pipefail

usage() {
	echo "usage: install.sh parent|kid" >&2
	exit 1
}

if [ "${1:-}" != "parent" ] && [ "${1:-}" != "kid" ]; then
	usage
fi
role=$1

if [ "$role" = "parent" ] && [ "$(id -u)" -eq 0 ]; then
	echo "Do not run parent install as root." >&2
	exit 1
fi

if [ "$role" = "kid" ] && [ "$(id -u)" -ne 0 ]; then
	exec sudo "$0" kid
fi

here=$(cd "$(dirname "$0")" && pwd)

repo=""
if [ -f "$here/manifest.json" ] && [ -f "$here/packaging/config.kid.toml" ]; then
	repo=$here
elif [ -f "$here/../manifest.json" ] && [ -f "$here/../packaging/config.kid.toml" ]; then
	repo=$(cd "$here/.." && pwd)
else
	echo "could not find Kidtimer (manifest.json + packaging)" >&2
	exit 1
fi

arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) want=kidtimer-linux-amd64 ;;
	aarch64 | arm64) want=kidtimer-linux-arm64 ;;
	*)
		echo "This computer is $arch. Need Linux x86_64 or aarch64." >&2
		exit 1
		;;
esac

bin=""
for base in "$here" "$repo"; do
	for name in "$want" kidtimer; do
		if [ -x "$base/$name" ]; then
			bin=$base/$name
			break 2
		fi
	done
done
if [ -z "$bin" ]; then
	echo "Missing $want" >&2
	exit 1
fi

if [ "$role" = "parent" ]; then
	dest=${HOME}/.local/share/kidtimer/src
else
	dest=/usr/local/share/kidtimer
fi

mkdir -p "$dest"
dest=$(cd "$dest" && pwd)
if [ "$repo" != "$dest" ]; then
	cp -a "$repo/packaging" "$dest/"
	cp -a "$repo/manifest.json" "$repo/BarWidget.qml" "$repo/Overlay.qml" "$dest/"
fi
if [ "$bin" != "$dest/kidtimer" ]; then
	cp -a "$bin" "$dest/kidtimer"
fi
chmod 755 "$dest/kidtimer"

if [ "$role" = "parent" ]; then
	"$dest/kidtimer" setup parent -repo "$dest"
else
	"$dest/kidtimer" setup kid -repo "$dest"
fi

echo
echo "Done. If the bar does not update: omarchy restart shell"
