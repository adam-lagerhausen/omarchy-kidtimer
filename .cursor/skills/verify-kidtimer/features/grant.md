# Grant time

Grant time applies a remaining delta through one HTTP call. Parent +10 / −10, CLI grant, testdata curl, and an app token are that call. Remaining is `MAX(0, remaining + seconds)`. A replayed idempotency key does not pay twice.

## Sub-features

- `grant-plus10` credits 600 seconds to `fun` with reason `+10`.
- `grant-minus10` posts parent −600 with reason `-10`. Remaining floors at 0. Sqlite stores seconds −600. Replay does not apply again. Not 8742.
- `grant-testdata` credits `fun` 900 with reason `good afternoon` via `testdata/grant.sh`.
- `grant-cli` credits through `kidtimer grant`.
- `grant-replay` returns the original result with `replay` true and does not add seconds.
- `grant-conflict` returns 409 when the same idempotency key is reused with a different body.

## How to get to it (user POV)

- Choose `+10` or `−10` on the parent tape. That credits the one clock (`fun`).
- Run `kidtimer grant -group fun -seconds 600 -reason "+10"`.
- Run `testdata/grant.sh` against a Stage B daemon.
- `POST /v1/grants` with a parent or app bearer, `Idempotency-Key`, and JSON `group`, `seconds`, `reason`. Parent may send a negative `seconds`. App debit is 403.

## Driving it with control-kidtimer

Preconditions:

- Isolated daemon is healthy. `control-kidtimer doctor` passed.
- `GET /v1/status` shows `groups.fun` at today's hours (3600 on a weekday).
- No grants row exists for idempotency key `testdata-grant-1` or `parent-minus10`.
- Do not use URL `http://127.0.0.1:8742`.

Numbers below assume a weekday seed of 3600. Saturday or Sunday seed is 7200; add 3600 to every remaining below.

- **Parent −10.** Debit fun 600 from remaining 3600. Run `.cursor/skills/verify-kidtimer/control-kidtimer grant --group fun --seconds -600 --reason "-10" --idempotency-key parent-minus10`. HTTP 200, `seconds` is -600, `reason` is `-10`, `source` is `parent`, `replay` is false, `remaining` is 3000.
- **Confirm floor path.** Read status. Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata status.sh`. `groups.fun` is 3000. `path_remaining.fun` is 3000. `path_remaining.school` is absent.
- **Minus10 ledger.** Run `.cursor/skills/verify-kidtimer/control-kidtimer sqlite "SELECT group_id, seconds, source, reason, idempotency_key FROM grants WHERE idempotency_key = 'parent-minus10'"`. One row. `seconds` is -600. `source` is `parent`. `reason` is `-10`.
- **Replay −10.** Run the same minus10 grant again. `replay` is true, `remaining` is still 3000, still one `parent-minus10` row.
- **Testdata grant.** Credit fun 900. Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata grant.sh`. HTTP 200, `group` is `fun`, `seconds` is 900, `reason` is `good afternoon`, `source` is `parent`, `replay` is false, `remaining` is 3900.
- **Confirm remaining.** Read status. Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata status.sh`. `groups.fun` is 3900. `path_remaining.fun` is 3900. `path_remaining.school` is absent.
- **Replay.** Run the same grant script again. Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata grant.sh`. `replay` is true and `remaining` is still 3900.
- **Conflict.** Reuse the testdata key with a different body. Run `.cursor/skills/verify-kidtimer/control-kidtimer http POST /v1/grants --body '{"group":"fun","seconds":1,"reason":"other"}' --idempotency-key testdata-grant-1`. HTTP 409.
- **Parent +10.** Credit another 600 with the panel body. Run `.cursor/skills/verify-kidtimer/control-kidtimer grant --group fun --seconds 600 --reason "+10" --idempotency-key parent-plus10`. `reason` is `+10`, `remaining` is 4500.
- **CLI grant.** Credit 15 more seconds. Run `.cursor/skills/verify-kidtimer/control-kidtimer cli -- grant -seconds 15 -reason "+15" -idempotency-key cli-plus15`. CLI defaults `-group` to `fun`. `remaining` is 4515.
- **Ledger side effect.** Read grants. Run `.cursor/skills/verify-kidtimer/control-kidtimer sqlite "SELECT group_id, seconds, source, reason, idempotency_key FROM grants ORDER BY id"`. Rows exist for `parent-minus10` (-600, parent, -10), `testdata-grant-1` (900, parent, good afternoon), `parent-plus10` (600, parent, +10), and `cli-plus15` (15, parent, +15). Replay did not insert a second `parent-minus10` or `testdata-grant-1` row.
- **Proof.** Save the minus10 JSON, status after that debit, the testdata grant JSON, status after that grant, the replay JSON, the 409 body, and the sqlite dump under `artifacts/grant/`. Status after the first testdata grant must show `fun` 3900. Sqlite `paused` is empty.

## Gotchas

- `testdata/grant.sh` hard-codes `IDEMPOTENCY_KEY` `testdata-grant-1`. A second run in the same sqlite is a replay, not a second credit.
- Missing `Idempotency-Key` is 400. The parent plugin always sends one.
- Clients must not send `source`. The daemon sets it from the token.
- CLI default `-seconds` is 900. The live test uses `-seconds 15` to distinguish CLI from testdata.
- Parent panel `+10` uses reason `+10`. `−10` uses reason `-10` and seconds -600. Testdata uses `good afternoon`. Assert the reason you sent.
- Clicking the live bar +10 talks to 8742 and the lab sqlite. That is not this recipe. Isolated proof is the same POST against the launched daemon.
- Today's hours refill at midnight. `+10` is today only. This isolated sqlite is fresh.
- App `POST` with negative `seconds` is 403. Zero is 400. Ask `seconds` must stay positive.
