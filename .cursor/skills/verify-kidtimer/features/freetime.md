# Free time until

Free time is a mode. Clocks do not run. Remaining is not rewritten. Until is part of ApplyMode, not a Look checkbox the plugin writes.

Default enter is until the next schedule change (covering block end, else next block, else end of local day). Minutes 15/30/60/120 set a wall-clock override. Sticky holds past covering blocks until the parent ends it.

## Sub-features

- `freetime-enter` posts `{id:freetime}` and leaves remaining unchanged. Status `mode` is `freetime`. `override_until` is set.
- `freetime-minutes` posts `{id:freetime, minutes:30}`. Status `override_until` is about 30 minutes ahead.
- `freetime-sticky` posts `{id:freetime, until:sticky}`. GET look `sticky_freetime` is true. Status omits `override_until`.
- `freetime-end` posts `{until:end}`. Mode returns to the covering schedule or null. Look `sticky_freetime` is false.
- `freetime-forbidden` rejects ask tokens with 403.

## How to get to it (user POV)

- Parent plugin free time button, then Until, then End free time.
- `POST /v1/mode` with a parent bearer.

## Driving it with control-kidtimer

Preconditions:

- Isolated daemon is healthy. `control-kidtimer doctor` passed.
- Do not use URL `http://127.0.0.1:8742`.

- **Enter.** POST `/v1/mode` body `{"id":"freetime"}`. HTTP 200. Status `mode` is `freetime`. Remaining matches the previous status. `override_until` is an RFC3339 timestamp.
- **Minutes.** POST `{"id":"freetime","minutes":30}`. Status `override_until` is ~30 minutes from now. GET look `sticky_freetime` is false.
- **Sticky.** POST `{"id":"freetime","until":"sticky"}`. GET look `sticky_freetime` is true. Status has no `override_until`.
- **End.** POST `{"until":"end"}`. Status `mode` is not `freetime`. GET look `sticky_freetime` is false.
- **Ask token.** POST `/v1/mode` with the ask token. HTTP 403.
- **Proof.** Save enter JSON, status after enter, minutes status, sticky look, end status, and the 403 under `artifacts/freetime/`.

## Gotchas

- Freetime does not rewrite remaining. A later evening apply still refills.
- `{id:freetime}` with no until is next schedule change, same override rule as morning.
- Minutes must be 15, 30, 60, or 120. Anything else is 400.
- parent-lab has no schedule windows, so next-change override is end of local day unless Look schedule has blocks.
