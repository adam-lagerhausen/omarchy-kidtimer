# Mint a token

The parent mints API keys on the kid daemon. The secret is printed once and stored hashed. Token kind is `parent`, `read`, `ask`, or `app`. App tokens default to group list `["fun"]` when that id exists.

## Sub-features

- `mint-ask` creates an ask token the kid panel uses for `POST /v1/asks`.
- `mint-read` creates a read token the kid bar uses for `GET /v1/status`.
- `mint-app` creates an app token that may `POST /v1/grants` within its caps.
- `mint-cli` mints through `kidtimer token create`.
- `mint-testdata` mints `khan-webhook` through `testdata/token-create.sh`.

## How to get to it (user POV)

- Run `kidtimer token create -name <name> -kind ask|read|app|parent`.
- Run `testdata/token-create.sh`.
- `POST /v1/tokens` with a parent bearer and JSON `name`, `kind`, optional `groups`, `max_seconds_per_grant`, `max_seconds_per_day`.

## Driving it with control-kidtimer

Preconditions:

- Isolated daemon is healthy. `control-kidtimer doctor` passed.
- Launch already minted `verify-ask` and `verify-read`. Do not reuse those names.
- No token named `khan-webhook` exists yet.

- **Testdata app mint.** Run `.cursor/skills/verify-kidtimer/control-kidtimer testdata token-create.sh`. HTTP 200. `name` is `khan-webhook`, `kind` is `app`, `groups` is `["fun"]`, `max_seconds_per_grant` is 600, `max_seconds_per_day` is 1800, `secret` is a non-empty string.
- **CLI ask mint.** Run `.cursor/skills/verify-kidtimer/control-kidtimer cli -- token create -name kid-bar -kind ask`. `kind` is `ask`. `secret` is present.
- **CLI read mint.** Run `.cursor/skills/verify-kidtimer/control-kidtimer cli -- token create -name bar-read -kind read`. Use that secret on `.cursor/skills/verify-kidtimer/control-kidtimer status --token <secret>`. HTTP 200, `kid_name` is `parent-lab`.
- **Scope.** App token cannot mint. Ask token cannot grant. Read token cannot ask. Prove one miss: `.cursor/skills/verify-kidtimer/control-kidtimer http POST /v1/asks --token read --body '{"group":"fun","seconds":60,"reason":"nope"}'`. HTTP 403, body `{"error":"forbidden"}`.
- **Ledger.** Run `.cursor/skills/verify-kidtimer/control-kidtimer sqlite "SELECT name, kind FROM tokens ORDER BY created_at"`. Names include `bootstrap`, `verify-ask`, `verify-read`, `khan-webhook`, `kid-bar`, `bar-read`. The `hash` column is not the secret.
- **Proof.** Save mint JSON (including `secret` only in the isolated artifact dir), a successful read-token status, a forbidden call body, and the sqlite name/kind dump under `artifacts/mint-token/`. Do not copy these secrets onto 8742 or into git.

## Gotchas

- Launch already used names `verify-ask` and `verify-read`. A second mint with the same name fails. Testdata uses `khan-webhook`. CLI recipes here use `kid-bar` and `bar-read`.
- Secret is shown once in the mint response. Later sqlite has only `hash`.
- `testdata/token-create.sh` omits `groups` and relies on default `fun`. Households without a `fun` group must name groups. This parent-lab config has `fun`.
- Parent tokens may omit `groups` and mean all. App tokens may not credit groups outside their list, and they cannot skip bedtime.
- Mint is parent-only. The kid plugin never calls `/v1/tokens`.
