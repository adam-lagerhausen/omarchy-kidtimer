# Read status

Status is the shared read model for the kid bar, parent bar, and CLI. It returns remaining seconds on the one clock (`fun`), spend-path remaining for that clock, bedtime flags, focused app, look version, pending ask count, leftover mode id, parent lock, remote lock, and bedtime clocks.

## Sub-features

- `status-parent` returns the full status with a parent token.
- `status-read` returns the same status with a read token.
- `status-cli` prints status through `kidtimer status`.
- `status-testdata` uses `testdata/status.sh`.
- `status-unauth` rejects missing or bad bearer with 401.
- `status-path` exposes `path_remaining.fun` (own remaining) and has no `school`, `minecraft`, or `youtube` keys.
- `status-bedtime-field` omits `bedtime_in` when `bedtime_lock` is false.
- `status-mode-lock` includes `mode` (string or null), `parent_locked` false on a fresh parent-lab ledger, `remote_lock` false, `bedtime_start`, `bedtime_end`, `look_version`, `focused_app` (null when idle), and `piles`.

## How to get to it (user POV)

- Kid bar widget `kidtimer.kid` polls `GET /v1/status` on localhost with a read token. Label is remaining (`47m`), `caption 47m` when focused, `bedtime`, or `locked`.
- Parent bar widget `kidtimer.parent` polls `GET /v1/status` with a parent token.
- Run `kidtimer status`.
- Run `testdata/status.sh`.

## Driving it with control-kidtimer

Preconditions:

- Isolated daemon is healthy. `control-kidtimer doctor` passed.
- Baseline remaining: `groups.fun` is today's hours (3600 on a weekday, 7200 Sat/Sun). No `minecraft`, `youtube`, or `school` clocks.
- Read token `verify-read` was minted at launch.

- **Unauth.** Call status with no bearer. Doctor already asserts this. To recapture it, source the run URL from `control-kidtimer env` and run `curl -sS -o /dev/null -w '%{http_code}' "$URL/v1/status"` with no `Authorization` header. HTTP 401.
- **Bad bearer.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http GET /v1/status --token nope`. HTTP 401, body `{"error":"unauthorized"}`.
- **Parent status.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata status.sh`. HTTP 200. `kid_name` is `parent-lab`. `groups.fun` is today's hours. `bedtime_active` is a boolean. `focused_group` is null. `pending_ask_count` is 0. JSON has no `bedtime_in` key. `parent_locked` is false. `mode` is null or `""`. `remote_lock` is false. `bedtime_start` is `21:00`. `bedtime_end` is `07:00`. `groups` has no `minecraft`, `youtube`, or `school` key.
- **Read token.** Run `.cursor/skills/verify-kidtimer/control-kidtimer status --token read`. Same `kid_name` and `groups.fun` as parent.
- **CLI.** Run `.cursor/skills/verify-kidtimer/control-kidtimer cli -- status`. Same JSON on stdout, exit 0.
- **Path remaining.** After a `fun` grant of 900 (see grant.md), run status again. `path_remaining.fun` is remaining after that grant. `path_remaining` has no `school` key.
- **Kid label contract.** Optional, not a user path: `node testdata/run-plugin-models.js` expects remaining / bedtime / locked strings. Do not count it as status proof.
- **Proof.** Save parent status JSON, read-token status JSON, CLI stdout, and the 401 body under `artifacts/status/`. Both successful bodies must show `kid_name` `parent-lab` and omit `bedtime_in`.

## Gotchas

- After 21:00 America/New_York, `bedtime_active` is true even with lock off. That is clock math, not a lock. Do not treat it as `omarchy system lock`. Kid bar would say `bedtime`. Asks still work unless bedtime lock is on.
- `bedtime_in` is omitted whenever `bedtime_lock` is false. Presence of that key means the wrong config.
- Ask tokens cannot `GET /v1/status`. Use read or parent.
- Live kid widget hard-codes `http://127.0.0.1:8742/v1/status`. Isolated status proof uses this run's URL, not 8742.
- `focused_group` stays null in Stage B. Live Hyprland focus is Stage E and is out of scope here.
- Leftover `POST /v1/mode` may set `mode` on status. It must not change `groups.fun`.
