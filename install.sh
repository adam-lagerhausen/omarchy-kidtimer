#!/bin/bash
set -euo pipefail

# Parent one-liner. Do not guess parent vs kid.
# curl -fsSL https://raw.githubusercontent.com/adam-lagerhausen/omarchy-kidtimer/master/install.sh | bash

usage() {
	echo "Say parent or kid. Example: curl -fsSL https://raw.githubusercontent.com/adam-lagerhausen/omarchy-kidtimer/master/install.sh | bash -s parent" >&2
	exit 1
}

if [ "$(uname -s)" != Linux ]; then
	echo "This is for Linux." >&2
	exit 1
fi

arch=$(uname -m)
case "$arch" in
	x86_64 | amd64) goarch=amd64 ;;
	aarch64 | arm64) goarch=arm64 ;;
	*)
		echo "This computer is $arch. Need Linux x86_64 or aarch64." >&2
		exit 1
		;;
esac

role=""
arg=${1:-}
if [ "$arg" = parent ] || [ "$arg" = kid ]; then
	role=$arg
elif [ -n "$arg" ]; then
	usage
else
	if [ ! -r /dev/tty ]; then
		usage
	fi
	echo "Which computer is this?" >/dev/tty
	echo "  1) Mine (the parent's)" >/dev/tty
	echo "  2) The kid's" >/dev/tty
	printf "Type 1 or 2: " >/dev/tty
	read -r pick </dev/tty
	case "$pick" in
		1) role=parent ;;
		2) role=kid ;;
		*)
			echo "Need 1 or 2." >&2
			exit 1
			;;
	esac
fi

if [ -z "${KIDTIMER_PACK_URL:-}" ]; then
	if ! command -v omarchy >/dev/null 2>&1 && [ ! -d "${HOME}/.config/omarchy" ]; then
		echo "This is for Omarchy." >&2
		exit 1
	fi
fi

if ! command -v curl >/dev/null 2>&1; then
	echo "Need curl." >&2
	exit 1
fi
if ! command -v sha256sum >/dev/null 2>&1; then
	echo "Need sha256sum." >&2
	exit 1
fi

pack_name=kidtimer-linux-$goarch.tar.gz
base=https://github.com/adam-lagerhausen/omarchy-kidtimer/releases/download/latest
pack_url=${KIDTIMER_PACK_URL:-$base/$pack_name}
sums_url=${KIDTIMER_SUMS_URL:-$(dirname "$pack_url")/SHA256SUMS}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Getting Kidtimer..." >&2
curl -fsSL -o "$tmp/$pack_name" "$pack_url"
curl -fsSL -o "$tmp/SHA256SUMS" "$sums_url"
if ! grep -E "  ${pack_name}$" "$tmp/SHA256SUMS" | (cd "$tmp" && sha256sum -c -); then
	echo "Checksum failed." >&2
	exit 1
fi

tar -xzf "$tmp/$pack_name" -C "$tmp"
dir=$tmp/kidtimer-linux-$goarch
if [ ! -x "$dir/install.sh" ]; then
	echo "The pack is missing install.sh." >&2
	exit 1
fi

if [ "$role" = parent ]; then
	if [ "$(id -u)" -eq 0 ]; then
		echo "Do not run parent install as root." >&2
		exit 1
	fi
	bash "$dir/install.sh" parent
else
	if [ "$(id -u)" -eq 0 ]; then
		bash "$dir/install.sh" kid
	else
		sudo bash "$dir/install.sh" kid
	fi
fi
