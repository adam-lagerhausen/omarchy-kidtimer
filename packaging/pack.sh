#!/bin/bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"
dist=$root/dist
mkdir -p "$dist"

pack_one() {
	local goarch=$1
	local name=kidtimer-linux-$goarch
	local out=$dist/$name
	rm -rf "$out"
	mkdir -p "$out"
	CGO_ENABLED=0 GOOS=linux GOARCH=$goarch GOTOOLCHAIN=go1.25.0 go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o "$out/$name" ./daemon/cmd/kidtimer
	/usr/bin/mkdir -p "$out/packaging"
	/usr/bin/find packaging -mindepth 1 -maxdepth 1 ! -name SHA256SUMS -exec /usr/bin/cp -a {} "$out/packaging/" \;
	for f in manifest.json BarWidget.qml Panel.qml Overlay.qml Setup.qml ParentPanel.qml KidPanel.qml ParentModel.js KidModel.js Tape.qml LookBtn.qml fonts icons; do
		if [ -e "$root/$f" ]; then
			/usr/bin/cp -a "$root/$f" "$out/"
		fi
	done
	/usr/bin/mkdir -p "$out/helpers"
	/usr/bin/tar -C "$root/helpers" --exclude='__pycache__' --exclude='*.pyc' -cf - . | /usr/bin/tar -C "$out/helpers" -xf -
	/usr/bin/cp packaging/install.sh "$out/install.sh"
	/usr/bin/chmod 755 "$out/install.sh" "$out/$name"
	/usr/bin/find "$out" -exec /usr/bin/touch -d '2024-01-01 00:00:00 UTC' {} +
	/usr/bin/tar --format=gnu --sort=name --owner=0 --group=0 --numeric-owner --mtime='UTC 2024-01-01' -C "$dist" -c "$name" | /usr/bin/gzip -n >"$out.tar.gz"
	echo "wrote $out.tar.gz"
}

pack_one amd64
pack_one arm64
(
	cd "$dist"
	/usr/bin/sha256sum kidtimer-linux-amd64.tar.gz kidtimer-linux-arm64.tar.gz >SHA256SUMS
)
echo "wrote $dist/SHA256SUMS"
# Local tar/gzip is not Ubuntu Actions. Commit packaging/SHA256SUMS from a
# pack workflow artifact, then this check on GITHUB_ACTIONS must match.
if [ -n "${GITHUB_ACTIONS:-}" ] && [ -f "$root/packaging/SHA256SUMS" ]; then
	if ! /usr/bin/cmp -s "$dist/SHA256SUMS" "$root/packaging/SHA256SUMS"; then
		echo "packaging/SHA256SUMS does not match this build" >&2
		/usr/bin/diff -u "$root/packaging/SHA256SUMS" "$dist/SHA256SUMS" >&2 || true
		exit 1
	fi
fi
