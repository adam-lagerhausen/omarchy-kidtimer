---
name: verify-kidtimer
description: Drive Kidtimer's grant API and CLI against an isolated Stage B daemon. Use when proving grants, status, asks, token mint, or the HTTP the Omarchy plugins send. Never drive the live :8742 lab or turn the enforcer on on the parent desk.
---

# Verify Kidtimer

Kidtimer is a bank daemon plus two Omarchy-shell plugins. The bank is the product. Plugins are HTTP clients of that bank.

Primary surface for this skill is the HTTP API on an isolated `kidtimer daemon`, plus the CLI subcommands that are HTTP clients of it (`grant`, `status`, `lock`, `unlock`, `asks`, `decide`, `token create`, `pin set`, `pin status`). Secondary surfaces, not driven here: the live kid and parent QML on the Omarchy bar, and the systemd unit.

Verification runs on the parent desk. Spec hard rules apply. Isolated verification never enables `enforcer`, never enables `bedtime_lock`, never sends `SIGSTOP`/`SIGCONT`, never calls `omarchy system lock`, and never talks to the live lab on `127.0.0.1:8742`.

## Launch

Use the helper. It builds `./daemon/cmd/kidtimer`, binds a free `127.0.0.1` port (not 8742), opens a fresh sqlite under `/tmp/kidtimer-verify-<id>/`, and starts the daemon with `setsid` so it outlives the helper process.

```bash
.cursor/skills/verify-kidtimer/control-kidtimer launch
.cursor/skills/verify-kidtimer/control-kidtimer doctor
```

Ready when `GET /v1/status` with no bearer returns 401, stderr contains `bootstrap parent token bootstrap (shown once): <secret>`, and doctor prints `doctor ok`. Launch also mints `verify-ask` and `verify-read` so kid paths can run.

Config is `packaging/config.parent-lab.toml`. `kid_name` is `parent-lab`. `enforcer = false`. `bedtime_lock = false`. `empty_lock = false`. A fresh ledger has `groups.fun` from today's `fun_hours` (3600 on a weekday).

Go may be missing from PATH in agent shells. The helper looks for `go` on PATH, then `mise which go`.

Do not start with `kidtimer daemon` on `:8742`. Do not use `$HOME/.local/share/kidtimer-lab/`. Do not use `/etc/kidtimer` or `/var/lib/kidtimer`. Do not `systemctl start|stop kidtimer`. Do not install `config.kid.toml` here.

Two isolated runs can coexist. Set `KIDTIMER_VERIFY_ID` to a second name. They still must not bind 8742.

Teardown is `control-kidtimer cleanup`. It SIGTERMs the pid it started, then deletes `/tmp/kidtimer-verify-<id>/`. It does not delete artifacts.

## Doctor

```bash
.cursor/skills/verify-kidtimer/control-kidtimer doctor
```

Doctor is read-only. It checks:

- The recorded pid is alive and its cmdline contains this run's `-data` and `-listen`.
- Listen is `127.0.0.1` and the port is not 8742.
- Sqlite is under `/tmp/kidtimer-verify-`, not the lab dir.
- Config still has `enforcer = false`, `bedtime_lock = false`, and `empty_lock = false`.
- Unauthenticated `GET /v1/status` is 401.
- Parent `GET /v1/status` is 200, `kid_name` is `parent-lab`, `bedtime_in` is omitted, and `parent_locked` is false. `mode` may be null or `""`.

If anything looks off, run doctor before driving. A failing doctor means stop. Do not fall back to 8742.

## Drive

Harness is `control-kidtimer`. Routes and bodies are the same ones the plugins send. Read `features/` before a run. Drive every listed entry point for the feature under test, not one convenient path.

Parent +10 / −10 (panel buttons `+10` / `−10`) is:

```bash
.cursor/skills/verify-kidtimer/control-kidtimer grant \
  --group fun --seconds 600 --reason "+10" --idempotency-key parent-plus10
```

That is `POST /v1/grants` with bearer parent token, header `Idempotency-Key`, body `{"group":"fun","seconds":600,"reason":"+10"}`. `−10` uses seconds `-600` and reason `-10`. Remaining is `MAX(0, remaining + seconds)`. Default group is `fun` when that id exists.

Checked-in curl scripts are a real user path too:

```bash
.cursor/skills/verify-kidtimer/control-kidtimer testdata grant.sh
.cursor/skills/verify-kidtimer/control-kidtimer testdata status.sh
.cursor/skills/verify-kidtimer/control-kidtimer testdata lock.sh
.cursor/skills/verify-kidtimer/control-kidtimer testdata mode.sh
.cursor/skills/verify-kidtimer/control-kidtimer testdata policy.sh
.cursor/skills/verify-kidtimer/control-kidtimer testdata ask.sh
.cursor/skills/verify-kidtimer/control-kidtimer testdata decide.sh <ask-id> approve
.cursor/skills/verify-kidtimer/control-kidtimer testdata token-create.sh
```

`grant.sh` always uses idempotency key `testdata-grant-1`. First call credits. Second call is a replay (`replay: true`) and must not add seconds.

CLI (same HTTP, not sqlite):

```bash
.cursor/skills/verify-kidtimer/control-kidtimer cli -- grant -group fun -seconds 15 -reason "+15" -idempotency-key cli-plus15
.cursor/skills/verify-kidtimer/control-kidtimer cli -- status
.cursor/skills/verify-kidtimer/control-kidtimer cli -- lock
.cursor/skills/verify-kidtimer/control-kidtimer cli -- unlock
.cursor/skills/verify-kidtimer/control-kidtimer cli -- asks
.cursor/skills/verify-kidtimer/control-kidtimer cli -- token create -name kid-bar -kind ask
```

Kid panel Ask is `POST /v1/asks` with the ask token. Parent Approve/Deny is `POST /v1/asks/{id}/decide` with `{"decision":"approve"}` or `"deny"`.

```bash
.cursor/skills/verify-kidtimer/control-kidtimer ask --group fun --seconds 900 --reason "one more video"
.cursor/skills/verify-kidtimer/control-kidtimer asks
.cursor/skills/verify-kidtimer/control-kidtimer decide <ask-id> approve
```

Raw route when a feature file names one:

```bash
.cursor/skills/verify-kidtimer/control-kidtimer http GET /v1/status --token read
.cursor/skills/verify-kidtimer/control-kidtimer http POST /v1/grants \
  --body '{"group":"fun","seconds":600,"reason":"+10"}' \
  --idempotency-key plugin-plus10
```

Do not click the live Omarchy bar. Those widgets talk to `http://127.0.0.1:8742` and the lab sqlite. Isolated proof is the HTTP they would send, against the daemon this run started.

`node testdata/run-plugin-models.js` checks QML helper labels. It is a contract test, not a user-path proof.

`go test ./...` is the unit gate (Stages A–C plus live testdata). This skill does not replace it. It proves the running daemon the way a parent token, ask token, or app token does.

## Evidence

Write proof under `.cursor/skills/verify-kidtimer/artifacts/<feature>/`. Cleanup must leave that directory.

A proof includes:

- The exact command (testdata script, CLI, or `control-kidtimer` invocation).
- The HTTP JSON body of the action.
- A second read of the result: `GET /v1/status` or `control-kidtimer status`.
- The sqlite side effect: `control-kidtimer sqlite "SELECT group_id, seconds, source, reason, idempotency_key FROM grants"` and, for asks, `SELECT id, group_id, seconds, status FROM asks`.
- `control-kidtimer doctor` output from before the drive.
- Feature id and which entry point produced the artifact.

Standards:

- Exercise real routes (`/v1/grants`, `/v1/status`, `/v1/asks`, `/v1/tokens`). Do not write balances by opening sqlite as the bank. Sqlite is the side-effect check after an HTTP grant.
- Capture the action and the resulting remaining seconds, not only the grant response.
- `source` on a grant row is set by the daemon from the token (`parent`, `ask:<id>`, `app:<name>`). Clients must not send `source`.
- Replay proof is `replay: true` plus unchanged `groups.fun`, plus still one grants row for that idempotency key.
- Same idempotency key with a different body must be HTTP 409.
- Do not treat `enforcer = false` as a name. Observe it: doctor requires the flag, and this run must never create rows in `paused` or signal another process. After a grant, `SELECT pid, group_id FROM paused` is empty.

Keep throwaway secrets out of git. Artifact JSON from an isolated run is fine on disk.

## Cleanup

```bash
.cursor/skills/verify-kidtimer/control-kidtimer cleanup
```

Kills only the pid recorded in `/tmp/kidtimer-verify-<id>/run.env`, and only if `/proc/<pid>/cmdline` contains that run's sqlite path. Then deletes the state dir. Does not kill by process name. Does not touch 8742. Does not delete `artifacts/`.

If a launch or drive fails, run cleanup before the next launch so ports and sqlite are not left behind.

## Helpers

`control-kidtimer` is this directory, executable. Invocation is in Launch/Doctor/Drive/Cleanup above. `control-kidtimer` with no args prints the command list.

`control-kidtimer env` prints run id, pid, url, data path, and whether tokens are set. It does not print secrets.

`control-kidtimer sqlite '<sql>'` reads the disposable ledger with a busy timeout while the daemon is up.
