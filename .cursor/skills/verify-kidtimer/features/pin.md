# Household parent PIN

A 4-digit household PIN unlocks the kid overlay and the parent lock square. It is not the login password. Set it from `kidtimer pin set` or parent settings. Until it exists, time still counts and nothing overlays.

## Sub-features

- `pin-set-http` stores the PIN on the kid daemon with parent `PUT /v1/parent-pin`.
- `pin-set-cli` hashes locally, writes a temp household file, and PUTs that hash.
- `pin-status` shows `parent_pin_set` true after set, never the PIN or hash.
- `pin-approve` approves a pending ask with ask token plus PIN.
- `pin-grant` credits `fun` with ask or read token plus PIN, source `parent-pin`. Isolated parent-lab adds seconds. On a kid box during bedtime it sets remaining to those seconds.
- `pin-wrong` rejects a bad PIN with 403 and does not grant.
- `pin-rate-limit` rejects further PIN tries after five failures.
- `pin-forbidden` rejects ask-token `PUT /v1/parent-pin` with 403.

## How to get to it (user POV)

- Parent settings: Parent PIN row, four boxes, Set. That runs `kidtimer pin set` with the digits on stdin.
- CLI: `kidtimer pin set` (TTY types twice; non-TTY one line on stdin). `kidtimer pin status`.
- Kid panel while Waiting: Parent Pin next to Ask, then a text field that approves that ask.
- Kid overlay Ask (−10 / +10) queues `/v1/asks`. Parent Pin, then minutes, then grant.
- HTTP: `PUT /v1/parent-pin`, `POST /v1/pin/approve`, `POST /v1/pin/grant`.

## Driving it with control-kidtimer

Preconditions:

- Isolated daemon is healthy. `control-kidtimer doctor` passed.
- Do not use URL `http://127.0.0.1:8742`.
- Do not write `~/.local/share/kidtimer/parent-pin`. CLI pin set must pass `-home` under `/tmp/kidtimer-verify-` and `-desk ''`.
- parent-lab keeps `enforcer`, `bedtime_lock`, `empty_lock`, and `remote_lock` false. Do not expect an overlay or `omarchy system lock` on this machine.

- **HTTP set.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http PUT /v1/parent-pin --body '{"pin":"1234"}'`. HTTP 200, `parent_pin_set` true.
- **Confirm status.** Run `.cursor/skills/verify-kidtimer/control-kidtimer status`. `parent_pin_set` is true. `overlay` is false. JSON does not contain the PIN or an argon2 hash.
- **CLI set.** Run `printf '4242\n' | .cursor/skills/verify-kidtimer/control-kidtimer cli -- pin set -home "$KIDTIMER_VERIFY_DIR" -desk ''`. Then `control-kidtimer cli -- pin status -home "$KIDTIMER_VERIFY_DIR"` prints `parent_pin_set`. Status on the daemon still `parent_pin_set` true.
- **Ask token cannot set.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http PUT /v1/parent-pin --token ask --body '{"pin":"9999"}'`. HTTP 403.
- **Pin approve.** Run `.cursor/skills/verify-kidtimer/control-kidtimer ask --group fun --seconds 120 --reason "more time"`. Then `.cursor/skills/verify-kidtimer/control-kidtimer http POST /v1/pin/approve --token ask --body '{"pin":"4242","ask_id":"<id>"}'`. HTTP 200, ask `status` is `approved`. `GET /v1/asks` is empty. Grant source is `ask:<id>`.
- **Wrong pin.** Create another ask. Run pin approve with `"pin":"0000"`. HTTP 403. Ask stays pending.
- **Pin grant.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http POST /v1/pin/grant --token ask --body '{"pin":"4242","seconds":60}'`. HTTP 200, `source` is `parent-pin`, `groups.fun` rose by 60. Sqlite `grants` has `source` `parent-pin` and `reason` `parent-pin`.
- **Rate limit.** POST `/v1/pin/grant` with `"pin":"0000"` five times using the ask token. The next try, even with `"pin":"4242"`, is HTTP 403 until the cooldown ends. Parent `POST /v1/grants` still works during that cooldown.
- **Proof.** Save set JSON, status after set, the 403 for ask PUT, approve JSON, grant JSON, grants sqlite, and a wrong-pin 403 under `artifacts/pin/`.

## Gotchas

- Isolated CLI pin set without `-home` would overwrite the live household PIN file. Always pass a temp `-home` and `-desk ''`.
- Parent bearer cannot call `/v1/pin/grant`. Kid overlay and Waiting Parent Pin use the ask (or read) token plus PIN.
- Overlay, bedtime freeze, and empty freeze stay off on the parent desk. This recipe proves the PIN API, not a session lock. Pin grant here adds 60 seconds. Bedtime SET-remaining is a kid-box overlay path (`bedtime_lock` on).
- Five failures start a short cooldown. Do not treat that 403 as a wrong-PIN proof for a later correct attempt until it lifts.
- Live bar PIN talks to 8742. Isolated proof uses this run's URL.
