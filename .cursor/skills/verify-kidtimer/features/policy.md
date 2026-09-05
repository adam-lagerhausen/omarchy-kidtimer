# Edit bedtime and mode minutes

Policy is a partial PATCH. The parent can change bedtime clocks, bedtime lock, and per-mode minutes without rewriting TOML. Edits live in sqlite overlay and survive daemon restart.

## Sub-features

- `policy-bedtime` sets `bedtime_start` to `20:00` and keeps `bedtime_end`.
- `policy-minutes` sets leftover evening `fun` overlay to 1800 without changing remaining. Applying evening afterwards still leaves remaining unchanged.
- `policy-testdata` uses `testdata/policy.sh`.
- `policy-forbidden` rejects ask tokens with 403.
- `policy-persist` keeps overlay values after the isolated daemon is killed and relaunched on the same sqlite.

## How to get to it (user POV)

- Parent plugin bedtime fields and per-group minute fields.
- Run `testdata/policy.sh` against a Stage B daemon.
- `PATCH /v1/policy` with a parent bearer and any of `bedtime_start`, `bedtime_end`, `bedtime_lock`, `modes`.

## Driving it with control-kidtimer

Preconditions:

- Isolated daemon is healthy. `control-kidtimer doctor` passed.
- Status `bedtime_start` is `21:00` and `bedtime_end` is `07:00` from TOML.
- Do not use URL `http://127.0.0.1:8742`.
- Do not set `bedtime_lock` true on this parent-lab run.

- **Testdata policy.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata policy.sh`. HTTP 200. `bedtime_start` is `20:00`. `bedtime_end` stays `07:00`. `modes.evening.fun` is 1800.
- **Confirm status.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata status.sh`. Same clocks. JSON still omits `bedtime_in`.
- **Apply evening.** Run `MODE=evening .cursor/skills/verify-kidtimer/control-kidtimer testdata mode.sh`. Status `groups.fun` is unchanged from before the PATCH.
- **Ask token.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http PATCH /v1/policy --token ask --body '{"bedtime_start":"20:00"}'`. HTTP 403.
- **Omitted fields stay.** A PATCH with only `bedtime_start` must leave `bedtime_end` and mode minutes as they were.
- **Proof.** Save the PATCH JSON, status after PATCH, status after evening apply, and the 403 body under `artifacts/policy/`. Sqlite meta has `bedtime_start` `20:00` and `mode_minutes` containing evening fun 1800.

## Gotchas

- Empty string on a clock means keep the current value. Omit a field to leave it alone.
- Overlay / Look hours never write remaining. ApplyMode does not refill. Midnight reads today's `fun_hours`.
- Sqlite meta has `look` JSON after first open. PATCH policy shims onto Look.
- parent-lab `bedtime_lock` stays false. Do not PATCH it true here.
- Restart proof uses this run's sqlite under `/tmp/kidtimer-verify-*`, never `$HOME/.local/share/kidtimer-lab/`.
