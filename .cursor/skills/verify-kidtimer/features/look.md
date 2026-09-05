# Look document

The Look is the parent-facing policy: bedtime and per-weekday hours. Remaining lives on the ledger. PUT Look does not credit or zero the clock. Membership lists do not spend.

## Sub-features

- `look-get` returns empty `piles` and a `freetime` mode (`kind: freetime`). Allotment modes from TOML have empty hours.
- `look-put-hours` writes evening fun hours without changing remaining (after piles exist on the document).
- `look-put-empty` writes empty `things` / `apps` so lists cannot come back. Remaining is unchanged.
- `look-forbidden` rejects ask tokens with 403.

## How to get to it (user POV)

- Parent plugin Mode Edit Save, Groups rows, Schedule track.
- `GET /v1/look` and `PUT /v1/look` with a parent bearer.

## Driving it with control-kidtimer

Preconditions:

- Isolated daemon is healthy. `control-kidtimer doctor` passed.
- Do not use URL `http://127.0.0.1:8742`.
- Do not set `bedtime_lock` true.

- **Get.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http GET /v1/look`. HTTP 200. `piles` is `[]`. `modes` includes `{id: freetime, name: Freetime, kind: freetime}`. `sticky_freetime` is false. Catalog entries have `id` and `name` only.
- **Status.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata status.sh`. `look_version` is a number. `groups.fun` is today's hours.
- **Put hours.** GET look, set leftover `modes` evening fun to 1800, PUT the document. Status `groups.fun` is still today's hours. Apply evening. Status `groups.fun` is still today's hours.
- **Ask token.** Run `.cursor/skills/verify-kidtimer/control-kidtimer http GET /v1/look --token ask`. HTTP 403.
- **Proof.** Save GET look, PUT body, status before apply, status after apply, and the 403 under `artifacts/look/`.

## Gotchas

- PUT Look never writes remaining. ApplyMode does not either.
- Seeded Look has no Games / YouTube / Fun piles. Parent adds groups. Pile ids are chosen at add time.
- An app sits in at most one pile. Unassigned catalog apps stay up. There is no sitting-around list.
- parent-lab still has `enforcer = false`. This recipe does not turn it on.
