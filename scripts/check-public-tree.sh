#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
needles=(
	Dropbox
	/home/adam
)
paths=(
	spec.md
	packaging
	plugin-parent
	plugin-kid
	README.md
	CONTRIBUTING.md
	SECURITY.md
	.cursor
	testdata
	docs/parent-look.md
	daemon
	.github
)
hit=0
for needle in "${needles[@]}"; do
	if rg -n -F -- "$needle" "${paths[@]}"; then
		hit=1
	fi
done
exit "$hit"
