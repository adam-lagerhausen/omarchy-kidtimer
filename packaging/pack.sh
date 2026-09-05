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
	CGO_ENABLED=0 GOOS=linux GOARCH=$goarch go build -trimpath -o "$out/$name" ./daemon/cmd/kidtimer
	cp -a plugin-parent plugin-kid packaging "$out/"
	cp packaging/install.sh "$out/install.sh"
	chmod 755 "$out/install.sh" "$out/$name"
	tar -C "$dist" -czf "$out.tar.gz" "$name"
	echo "wrote $out.tar.gz"
}

pack_one amd64
pack_one arm64
(cd "$dist" && sha256sum kidtimer-linux-amd64.tar.gz kidtimer-linux-arm64.tar.gz >SHA256SUMS)
echo "wrote $dist/SHA256SUMS"
