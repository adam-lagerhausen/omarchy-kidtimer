# Ask and decide

A kid ask queues a request and does not credit time. The parent approves or denies. Approve performs one grant whose source is `ask:<id>`. Deny records the ask as denied. Either way the ask leaves pending. The asked group is the one clock (`fun`).

## Sub-features

- `ask-create` queues a pending ask from the kid panel or `testdata/ask.sh`.
- `ask-list` shows pending asks to the parent.
- `ask-approve` credits the asked group and seconds, source `ask:<id>`.
- `ask-deny` leaves remaining unchanged and sets status `denied`.
- `ask-second-decide` returns 409 `already decided` and does not grant again.

## How to get to it (user POV)

- Kid panel: `Ask` (auto-opens at 0 remaining), choose minutes, choose `Ask`. Cancel discards the sheet.
- Parent tape: a pending card with Deny and Approve. New ids also fire an Omarchy notification.
- Run `testdata/ask.sh`, then `testdata/decide.sh <id> approve`.
- `kidtimer asks` and `kidtimer decide <id> approve` as HTTP clients.
- `POST /v1/asks` with an ask token. `GET /v1/asks` with a parent token. `POST /v1/asks/{id}/decide` with `{"decision":"approve"}` or `"deny"`.

## Driving it with control-kidtimer

Preconditions:

- Isolated daemon is healthy. `control-kidtimer doctor` passed.
- Ask token `verify-ask` was minted at launch.
- `groups.fun` is today's hours (3600 on a weekday). `pending_ask_count` is 0.
- `GET /v1/asks` returns `{"asks":[]}`.

Numbers below assume a weekday seed of 3600. If this run already applied the grant recipe, remaining starts at 4515 and approve lands at 5415.

- **Kid ask.** Queue more time. Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata ask.sh`. HTTP 200. `group` is `fun`, `seconds` is 900, `reason` is `one more video`, `status` is `pending`, `id` is a non-empty string. Record that id.
- **Status pending.** Run `.cursor/skills/verify-kidtimer/control-kidtimer status`. `pending_ask_count` is 1. `groups.fun` is unchanged.
- **Parent list.** Run `.cursor/skills/verify-kidtimer/control-kidtimer asks`. `asks` has one object whose `id`, `group`, and `seconds` match the create response. Keys are lowercase `id`, not `ID`.
- **Approve.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata decide.sh <ask-id> approve`. `status` is `approved`. Nested `grant.group` is `fun`, `grant.seconds` is 900, `grant.source` is `ask:<ask-id>`.
- **Confirm credit.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata status.sh`. `groups.fun` is 4500 from a fresh weekday seed (or 5415 after the grant recipe). `pending_ask_count` is 0.
- **Second decide.** Run `.cursor/skills/verify-kidtimer/control-kidtimer decide <ask-id> approve`. HTTP 409, body contains `already decided`. Fun remaining stays as after the first approve.
- **Deny path.** Create a second ask. Run `.cursor/skills/verify-kidtimer/control-kidtimer ask --group fun --seconds 60 --reason "more time"`. Then `.cursor/skills/verify-kidtimer/control-kidtimer decide <id> deny`. `status` is `denied`. `groups.fun` is unchanged. Pending count returns to 0.
- **Ledger.** Run `.cursor/skills/verify-kidtimer/control-kidtimer sqlite "SELECT id, group_id, seconds, status FROM asks ORDER BY created_at"`. First row `fun` 900 `approved`. Second row `fun` 60 `denied`. Grants include a row with `source` `ask:<first-id>` and `group_id` `fun`.
- **Proof.** Save create JSON, list JSON, approve JSON, status after approve, 409 body, deny JSON, and the sqlite dump under `artifacts/ask-decide/`.

## Gotchas

- `testdata/ask.sh` uses the ask token. `control-kidtimer testdata` already switches `KIDTIMER_TOKEN` to `ASK_TOKEN` for `ask.sh`. Other scripts use the parent token.
- Parent tokens cannot `POST /v1/asks`. Ask tokens cannot decide.
- Kid panel reason is `more time`. Testdata reason is `one more video`. Assert the reason you sent.
- Kid panel default ask is 30 min (1800). Testdata is 900.
- Approve is a grant. It needs no `Idempotency-Key` on the decide route. The bank stores one.
- Live kid panel posts to hardcoded `http://127.0.0.1:8742/v1/asks`. Isolated proof uses this run's URL.
- Parent notification on a new ask id is Stage D/E visual. Isolated proof is the pending list JSON and the id appearing.
- Empty remaining does not block asks. Bedtime and parent lock do.
