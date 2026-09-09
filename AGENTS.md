# Project agent memory

This file is the project's committed home for project-intrinsic agent knowledge: build, test, release, architecture, and sharp-edge notes that should travel with the code.

- `go test ./...` from the repo root is the bar. CONTRIBUTING.md.
- Kid role install must not `sudo` copy from a user-owned temp. The live path is `helpers/apply-role.sh`; the tarball path is `packaging/install.sh`. Both call `helpers/install-lib.sh`, which hashes first and pipes `helpers/privileged-install.py` to root on stdin. Parent install must not run as root.

## Maintaining this file

Keep this file for knowledge useful to almost every future agent session in this project.
Do not repeat what the codebase already shows; point to the authoritative file or command instead.
Prefer rewriting or pruning existing entries over appending new ones.
When updating this file, preserve this bar for all agents and keep entries concise.
