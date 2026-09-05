# Apply a mode

Leftover HTTP. `POST /v1/mode` still records an active mode id on status. It must not change remaining. The daily timer ignores modes.

## Sub-features

- `mode-morning` applies id `morning` and leaves `groups.fun` unchanged.
- `mode-evening` applies `evening` after morning and still leaves remaining unchanged.
- `mode-replay` posts the same id again and does not refill remaining.
- `mode-testdata` uses `testdata/mode.sh`.
- `mode-forbidden` rejects ask tokens with 403.

## How to get to it (user POV)

- There are no parent mode buttons. This route is leftover API.
- Run `testdata/mode.sh` against a Stage B daemon. Default id is `morning`. Set `MODE=evening` for evening.
- `POST /v1/mode` with a parent bearer and JSON `id`.

## Driving it with control-kidtimer

Preconditions:

- Isolated daemon is healthy. `control-kidtimer doctor` passed.
- Baseline remaining: `groups.fun` is today's hours (3600 on a weekday).
- `mode` on status may be null until the first apply. parent-lab has no `default_mode`.
- Do not use URL `http://127.0.0.1:8742`.

- **Testdata morning.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata mode.sh`. HTTP 200, `id` is `morning`.
- **Confirm remaining.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata status.sh`. `mode` is `morning`. `groups.fun` is still today's hours.
- **Replay.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata mode.sh` again. HTTP 200. Remaining is unchanged. A grant in between is kept.
- **Evening.** Run `MODE=evening .cursor/skills/verify-kidtimer/control-kidtimer testdata mode.sh`. Status `mode` is `evening`. `groups.fun` is still unchanged.
- **Ask token.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http POST /v1/mode --token ask --body '{"id":"morning"}'`. HTTP 403.
- **Proof.** Save morning JSON, status after morning, evening JSON, status after evening, and the 403 body under `artifacts/mode/`. After evening, remaining must match the number from before the first apply.

## Gotchas

- Mode enter is not a remaining reset. Midnight reads today's `fun_hours`, not the leftover mode table.
- parent-lab launch does not apply a schedule window. `mode` stays null until this recipe.
- Live proof uses this run's URL, not 8742.
