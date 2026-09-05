# Kidtimer verification map

This directory is the maintained source for verifying user-facing Kidtimer behavior. Read this index before driving, then use the matching feature file as the recipe.

## Baseline preconditions

- Launch with `.cursor/skills/verify-kidtimer/control-kidtimer launch`. That starts `kidtimer daemon` on a free `127.0.0.1` port with `packaging/config.parent-lab.toml` and sqlite under `/tmp/kidtimer-verify-$KIDTIMER_VERIFY_ID`.
- Run `control-kidtimer doctor` and require `kid_name=parent-lab`, `enforcer=false`, `bedtime_lock=false`, `empty_lock=false`, listen port not 8742, unauthenticated status 401.
- Fresh ledger: `groups.fun` is today's hours (3600 on a weekday, 7200 Sat/Sun), no minecraft/youtube/school clocks, `pending_ask_count` is 0, `bedtime_in` is omitted, `parent_locked` is false, `mode` may be null or `""`.
- Never drive `http://127.0.0.1:8742` or `$HOME/.local/share/kidtimer-lab/`. That is the live lab, not this run.
- Never enable the enforcer or bedtime lock. Never `systemctl` the kidtimer unit. Never signal sshd, Tailscale, Hyprland, omarchy-shell, Cursor, or a daily browser.

## Driving conventions

- Start every recipe from the baseline unless its preconditions say otherwise.
- Treat commands as literal. Keep script names, JSON keys, and idempotency keys unchanged when the file names them.
- HTTP bodies and routes are the plugin wire. Parent +10 is `POST /v1/grants` with `reason` `+10` and 600 seconds. Parent −10 is the same route with `reason` `-10` and -600 seconds. Kid Ask is `POST /v1/asks`. Parent Approve is `POST /v1/asks/{id}/decide`.
- `control-kidtimer testdata <script>` runs the checked-in curl in `testdata/` against this run's URL and token.
- `control-kidtimer cli -- ...` runs the built `kidtimer` binary as an HTTP client. It does not open sqlite.
- Restore nothing after a mutation except as a feature file says. Isolated sqlite is disposable. Proof artifacts are not.

## Proof and skip reporting

- Capture the user action and the resulting `GET /v1/status`, not only the mutating response.
- Mutation proof includes a sqlite read of `grants` or `asks`.
- Record the feature id and entry point on every artifact under `.cursor/skills/verify-kidtimer/artifacts/<feature>/`.
- Report an unreachable entry point with the command and the unmet precondition. Do not call a skipped plugin-bar click verified because HTTP succeeded. The bar click is listed so later Stage E runs can cover it with eyes on the screen.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph describing the user-visible behavior. It then uses exactly four H2 sections in this order.

1. `Sub-features` lists short IDs with one line for each behavior.
2. `How to get to it (user POV)` lists every user entry point.
3. `Driving it with control-kidtimer` starts with `Preconditions:` and uses labeled bullets that pair each user action with an exact command and observable result.
4. `Gotchas` lists traps that can waste or invalidate a verification run.

Keep implementation details out of the map. Name only user paths, stable handles, required state, commands, and observable proof.

## Features

- [Grant time](./grant.md) covers parent +10/−10, CLI grant, testdata grant, replay, and 409 on a reused key with a different body.
- [Read status](./status.md) covers remaining seconds, path remaining, read-token status, and CLI status.
- [Ask and decide](./ask-decide.md) covers kid asks, pending list, approve credit, deny, and second-decide 409.
- [Mint a token](./mint-token.md) covers parent mint of ask, read, and app tokens, secret shown once, app default group `fun`.
- [Parent lock](./lock.md) covers `POST /v1/lock`, replay, unlock, and 403 for ask tokens.
- [Household parent PIN](./pin.md) covers `PUT /v1/parent-pin`, CLI pin set/status, pin approve, pin grant, rate-limit, and ask-token 403.
- [Edit bedtime](./policy.md) covers `PATCH /v1/policy` bedtime clocks that do not refill remaining, and sqlite persist.
- [Look document](./look.md) covers GET/PUT `/v1/look` and PUT that does not change remaining.
