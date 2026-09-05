# Parent lock

Parent lock is a session freeze the parent turns on from the plugin or HTTP. It is stored on the kid daemon. `remote_lock` in config decides whether the enforcer actually locks. parent-lab keeps `remote_lock` false so this flag cannot freeze the parent session.

## Sub-features

- `lock-on` sets `parent_locked` true with `POST /v1/lock` `{"locked":true}`.
- `lock-replay` repeats the same body and stays 200, still locked.
- `lock-off` sets `parent_locked` false.
- `lock-testdata` uses `testdata/lock.sh`.
- `lock-forbidden` rejects ask, read, and app tokens with 403.
- `lock-unauth` rejects a bad bearer with 401.

## How to get to it (user POV)

- Parent plugin Lock / Unlock on the selected kid.
- Run `testdata/lock.sh` against a Stage B daemon.
- `kidtimer lock` / `kidtimer unlock` as HTTP clients (Direct url+token on Stage B, desk loopback on the parent tape).
- `POST /v1/lock` with a parent bearer and JSON `locked`.

## Driving it with control-kidtimer

Preconditions:

- Isolated daemon is healthy. `control-kidtimer doctor` passed.
- `GET /v1/status` shows `parent_locked` false.
- Do not use URL `http://127.0.0.1:8742`.
- parent-lab config keeps `remote_lock` false. Do not expect a session lock on this machine.

- **CLI lock.** Run `.cursor/skills/verify-kidtimer/control-kidtimer cli -- lock`. HTTP 200, `locked` is true. Status `parent_locked` is true.
- **CLI unlock.** Run `.cursor/skills/verify-kidtimer/control-kidtimer cli -- unlock`. HTTP 200. Status `parent_locked` is false.
- **Testdata lock.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata lock.sh`. HTTP 200, `locked` is true.
- **Confirm status.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata status.sh`. `parent_locked` is true. `bedtime_active` is unchanged from doctor. JSON still omits `bedtime_in`.
- **Replay.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata lock.sh` again. HTTP 200, `locked` is true. Status still `parent_locked` true.
- **Unlock.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http POST /v1/lock --body '{"locked":false}'`. HTTP 200. Status `parent_locked` is false.
- **Ask token.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http POST /v1/lock --token ask --body '{"locked":true}'`. HTTP 403.
- **Bad bearer.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http POST /v1/lock --token nope --body '{"locked":true}'`. HTTP 401.
- **Proof.** Save the first lock JSON, status after lock, the replay JSON, unlock JSON, and the 403 body under `artifacts/lock/`. Sqlite meta `parent_lock` is `true` after lock-on and `false` after unlock.

## Gotchas

- Lock on the parent desk does not freeze the session. `remote_lock` is false. The flag still shows in status.
- Replay does not need an `Idempotency-Key`. Same body twice is 200.
- Ask, read, and app tokens cannot lock. Read may GET the new status field.
- Live bar Lock talks to 8742. Isolated proof uses this run's URL.
