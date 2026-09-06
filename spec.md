# Kidtimer

Kidtimer enforces kids' computer screen time on Omarchy. The parent desk runs a control panel. Apps grant time through the grant API. A plugin is the UI. A privileged daemon on the kid machine is the bank.

The repo is for other Omarchy parents. It is not a HomeschoolOS feature. Any app with a scoped token can grant the same way.

This is not `ol4vr/omarchy-plus-screen-time`. That plugin tracks focus and draws a donut. Do not compete on charts. This project enforces limits, gives the parent a control panel, and exposes a grant API.

## Household

The parent Omarchy 4 desk runs the parent plugin. It is a ledger lab, not a kid box.

Kid machines, one kid each, dual-boot Windows and Omarchy. Example hostnames:

- `kid-a`
- `kid-b`

Windows is the escape hatch. Kidtimer only counts time while Omarchy is running. Dual-boot Family Safety on the Windows side is none of this project's business.

v1 is a daily computer timer plus bedtime. A 9-year-old who kills the bar is fine. Kid accounts may have sudo. The daemon still runs as a systemd system unit so killing the shell does not credit time. `sudo systemctl stop kidtimer` will stop the bank. Document that. Do not spend v1 fighting it.

Using the computer spends the one clock. Always-on system UI and idle do not. Fun vs School lists are gone.

## SSH and parent-desk safety

A parent often works on the parent desk over SSH, including from away. Locking the graphical session or freezing the wrong process is how you strand them.

The parent desk is a ledger lab, not a kid box.

Hard rules for anything that runs on the parent desk:

- Do not enable the enforcer on the parent desk. Enforcer defaults off. Ledger and HTTP may run. No `SIGSTOP`, no `SIGCONT`, no `omarchy system lock`, no Kidtimer overlay.
- Bedtime lock defaults off. A 21:00 config must not overlay the parent desk just because the clock says so.
- `remote_lock` defaults off. Parent lock is a flag on the ledger. It must not overlay the graphical session on the parent desk.
- If the enforcer is on, lab mode is required. `lab.enabled` must be true. The only processes it may pause are windows whose class is in `lab.classes`. Unknown windows are ignored, not frozen.
- Never signal `sshd`, `tailscaled`, `systemd`, Hyprland, `omarchy-shell`, or the SSH session. Match by window class of a toy app, not by "every process owned by the logged-in user".
- Kill switch from SSH is `sudo systemctl stop kidtimer`. That must keep working. Do not freeze sshd to "test pause".
- Do not install a kid-machine config onto the parent desk. Packaging ships `config.parent-lab.toml` and `config.kid.toml` as separate files. `advertise` stays off on the parent lab.

Rungs 1 and 2 on the parent desk are the only testing allowed until someone is sitting at a dual-boot kid machine.

## Split

| Piece | Where it runs | Job |
| --- | --- | --- |
| Kid daemon | systemd system unit on the kid Omarchy install | Ledger, remaining time, lock and pause even if the shell is dead |
| Kid plugin | QML in `omarchy-shell` on the kid session | Countdown on the bar, request-more-time panel, fullscreen overlay |
| Parent plugin | QML on the parent desk | Remaining time per kid, +10 / −10, approve or deny asks |
| Grant API | Same daemon | Parent grants, ask-approvals, and third-party apps all credit a group |

Do not put balances on the parent laptop. Do not enforce only inside Quickshell. Do not edit `/usr/share/omarchy/`. User plugins live in `~/.config/omarchy/plugins/`.

```
[parent plugin on parent desk] --LAN mDNS + HTTP-->  [kid daemon = ledger + HTTP]
[any app with a token] --Bearer------------>  [same API]
[Minecraft / browser]                        [tracker + enforcer on kid box]
[kid bar widget]                             [read-only view of the ledger]
```

One daemon, one API, callers with different keys.

Install is two commands: `kidtimer setup parent` on the parent desk, `sudo kidtimer setup kid` on the kid box. Same LAN is enough. The kid advertises `_kidtimer._tcp`. The parent claims the first unseen machine with `POST /v1/pair` and remembers it in `~/.local/share/kidtimer/kids.json`. No token paste. No Tailscale name list. Set the household 4-digit parent PIN with `kidtimer pin set` or parent settings before lock or overlay can fire.

If the parent laptop is asleep, enforcement still works. Request-more-time waits until the parent desk is back. No always-on hub in v1. No Tailscale Funnel. No public internet. Do not bind `0.0.0.0` on a public address.

## Unlocking is one call

Credit a group: kid, group, seconds, source, idempotency key.

Kid is implied by which daemon you called. Source is derived from the token, not sent by the client. Parent +10 / −10, approve-request, and a math app webhook are that call. Do not add a second credit path. Do not call this an Omarchy hook. Omarchy already has `theme-set` and `post-boot`. This is a grant API.

## Look and ledger

The kid daemon stores two things. **Look** is the policy document (`meta.look` JSON). **Ledger** is live remaining, lock, focus, and leftover mode id. PUT Look cannot write remaining. POST `/v1/mode` may record an id on status. It does not copy that mode's hours onto remaining.

Look holds bedtime and per-weekday hours. Seed is one clock id `fun` (the ledger name; the panel does not say Fun) and weekday hours (1h Mon–Fri, 2h Sat–Sun). Membership lists do not spend. Modes and schedule do not drive remaining.

A Look has piles, leftover `things` / `apps` / `matchers` (ignored for spend), `pile_hours`, `fun_hours`, leftover named modes, a leftover schedule, and bedtime `{lights_out, duration}` with wake derived. Midnight refills remaining from today's `fun_hours`.

## Groups

One screen-time number. After seed the live clock is `fun`. Parent copy does not say Fun or School.

TOML names the one metered group for first-open seed and for the parent-lab matcher:

| Field | Meaning |
| --- | --- |
| `id` | String key in config, ledger, and API. Seed keeps `fun` |
| `policy` | `metered` |
| `daily_seconds` | Seed default if Look hours are missing |
| `match` | parent-lab fallback only. Kid spend does not use match lists |
| `overflow` | Parsed for old files. The bank ignores it |

`metered` has remaining seconds. At midnight, remaining is set to that day's `fun_hours`. Extra grants from yesterday do not stack. Parent tokens may credit above the allotment for today. Modes and schedule do not change remaining.

Parent edits of Look hours and bedtime persist in sqlite `meta.look`. `empty_lock`, bedtime lock, and remote lock stay TOML / overlay flags. One kid per machine. The daemon on that machine is the bank.

Generic parent +10 and app tokens default to crediting `fun`.

v1 household clock after seed:

| id | display | daily_seconds | At zero |
| --- | --- | --- | --- |
| `fun` | (minutes left) | 3600, or that day's `fun_hours` | overlay when `empty_lock` is on and a parent PIN is set |

Bedtime is a hard window, default 21:00 to 07:00 local. Leftover minutes do not matter after bedtime unless a Parent Pin grant or an approved ask during bedtime set remaining to the granted minutes. That stay-up lasts until those minutes run out, then the bedtime overlay returns. Bedtime shows the Kidtimer overlay. It does not call `omarchy system lock`. It is not mixed into the clock.

One household parent PIN, four digits. Not the login password and not sudo. `kidtimer pin set` (or parent settings, which invokes that CLI) hashes it with argon2id, writes `~/.local/share/kidtimer/parent-pin` mode `0600`, and `PUT`s the same hash to every kid daemon. Status only says `parent_pin_set`. No PIN set means fail open: time still counts, no overlay, parent lock square does nothing.

Parent +10 / −10 applies a 600-second remaining delta to a chosen group (reasons `+10` / `-10`). Default `fun` when that id exists. Remaining is `MAX(0, remaining + seconds)`. Parent may debit. App may not.

App tokens may credit a configured group list. Default `["fun"]` if that id exists. Otherwise the mint must name groups.

## Always-on and never-signal

Always-on is a class list, not a group. The bar must not report focused group `system` when the kid is on the desktop. If the focused class is in `always_on`, treat it as no match.

`always_on` includes the shell, bar, lock, launcher, notifications, and polkit. Empty class, meaning the desktop, is always-on.

`never_signal` is a process-name list. Never `SIGSTOP` or `SIGCONT` these, even if a match says to. v1: `sshd`, `tailscaled`, `systemd`, `Hyprland`, `omarchy-shell`.

## Match and enforce

The enforcer polls about once a second when `enforcer = true`. Production focus source is Hyprland. Tests inject a fake compositor.

Each tick:

1. If a parent PIN is set and `(bedtime_lock and bedtime is active and not (stay-up hold and remaining > 0)) or (remote_lock and parent_lock)`, freeze: do not decrement, do not call `omarchy system lock`. The kid plugin shows the overlay. Bedtime window wraps midnight. 21:00 to 07:00 is inside. Overlay is a window, not a pulse. Parent lock on the parent desk does not freeze because `remote_lock` is false. Stop.
2. If a parent PIN is set and `empty_lock` is on, lab is off, and remaining is 0, freeze the same way. Do not set `parent_locked`. Asks stay allowed. Stop. The parent lab keeps `empty_lock = false`.
3. If the session is locked, or idle for 60 seconds, do not decrement. Idle source is `loginctl IdleHint` or Hyprland idle. Stop.
4. Read focused class, title, and pid. If Hyprland is down, skip the sample. Do not crash. Do not overlay. Stop.
5. If class is in `always_on`, or class is empty, ignore. Stop.
6. If `lab.enabled` and the class is not in `lab.classes`, ignore. Stop.
7. Spend the `fun` clock. If remaining is greater than zero, decrement one second. If remaining is zero and lab is on, `SIGSTOP` the focused pid unless its process name is in `never_signal`. Kid boxes overlay at step 2 instead of pausing apps.

`enforcer = false`: you may decrement in the ledger for dry-run logs. Never send signals. Never overlay. `bedtime_lock = false` skips the bedtime freeze even if the clock is inside the window. `remote_lock = false` skips freeze even if `parent_lock` is set. `empty_lock = false` skips freeze at zero. No parent PIN skips all three freezes; time still counts.

`SIGCONT` after a grant to `fun` resumes recorded paused lab pids for that clock.

v1 does not refuse launch. Empty, bedtime, and parent lock show the Kidtimer overlay on a kid box when a parent PIN is set. Never `omarchy system lock`. Reboot, TTY, and `sudo systemctl stop kidtimer` stay the parent hatch. If the kid kills the shell, fail open: overlay is gone, Super binds return.

Asks are allowed at empty remaining, bedtime, and parent lock. Approve outside bedtime adds the asked seconds. Approve during bedtime (lock on) sets remaining to the asked minutes and stay-up until those minutes run out. It does not clear parent lock.

Do not count time in the parent plugin. The daemon samples focus on the kid box.

## Tracking

Apps: focused window class and title from Hyprland (`hyprctl activewindow -j`). Do not count idle, lock, always-on classes, or the empty desktop. Any other focused window spends the one clock. Minecraft, Khan, Chrome, and a terminal are the same.

Sites do not get a separate clock. A browser window spends like any other window.

## Trust

Anything running as the child is hostile. A page in the kid's browser must not grant time.

Tokens live on the kid daemon. `setup kid` mints the local read and ask tokens for the kid bar. The first parent on the LAN claims a parent token through `POST /v1/pair`. The CLI can still mint more. Hashed at rest. Shown once.

Token `kind` is `parent`, `read`, `ask`, or `app`.

Token scope:

- which kid / machine
- which groups it may credit. App default `["fun"]` if that id exists
- max seconds per grant
- max seconds per day
- parent tokens can credit any group and exceed allotment. They still cannot skip bedtime
- app tokens cannot lift bedtime or raise caps
- read tokens may `GET /v1/status` only
- ask tokens may `POST /v1/asks` and PIN routes `POST /v1/pin/approve` and `POST /v1/pin/grant`
- read tokens may PIN routes as well. They still cannot set the PIN.

Stolen app token equals that token's daily cap. The log will show it. That is the v1 blast radius.

Idempotency keys on every grant. Retries must not double-pay.

Append-only log, example: `fractions lesson 12 → +10 fun at 16:02`.

A kid "I finished the worksheet" is an ask, not a grant. The parent approves, and approve is a grant.

## HTTP API

Listen is the bind set from `-listen` or config `listen` (comma-separated). Default `127.0.0.1:8742`. Kid packaging ships `listen = "0.0.0.0:8742"` so the parent can reach IPv4 on the LAN. `advertise` is mDNS only; it does not add extra bind addresses. A failed address is skipped. IPv6 link-local binds with the `%iface` zone. Need at least one successful listen. parent-lab and isolated verify stay `127.0.0.1`. No TLS in v1. Bearer tokens are the API identity except `POST /v1/pair`.

A kid box with `advertise = true` announces `_kidtimer._tcp` with TXT `id` (stable `/var/lib/kidtimer/machine-id`) and `name` (`kid_name`).

All other mutating routes require `Authorization: Bearer <token>`.

`kidtimer lock`, `kidtimer unlock`, `kidtimer grant`, `kidtimer status`, `kidtimer asks`, and `kidtimer decide` are HTTP clients. They talk to the parent desk on loopback, or to a kid daemon when both a URL and a token are set. `kidtimer token create` and `kidtimer pin set` (against a daemon URL) are HTTP clients of the running daemon. They do not open sqlite. The daemon is the only writer of the ledger. `kidtimer pin set` also writes the household hash file on the parent desk.

### `POST /v1/grants`

```json
{
  "group": "fun",
  "seconds": 900,
  "reason": "good afternoon"
}
```

Header `Idempotency-Key` required. Replay with the same key and the same body returns the original result. Same key with a different body returns 409.

Do not send `source`. The daemon sets it from the token: `parent`, `ask:<id>`, or `app:<token-name>`. Store that on the ledger row.

A token may only credit groups in its list. Parent tokens may credit any group.

### `GET /v1/status`

Remaining seconds (`groups.fun`), spend-path remaining (`path_remaining.fun`), whether bedtime is active, focused clock `fun` or none (`focused_group`), caption or none (`focused_app`), `spent.fun` seconds today, `today` (spans of time on the computer: `kind`, `start` minutes from midnight, `dur` minutes, `label`), `look_version`, pending ask count, leftover `mode` (ignored for remaining), `parent_locked`, `remote_lock`, `parent_pin_set`, `overlay` (true when the kid plugin should cover the screen), `bedtime_hold` (stay-up while remaining is greater than zero), effective `bedtime_start` and `bedtime_end` as `HH:MM`. When effective `bedtime_lock` is on, also `bedtime_in`: seconds until the bedtime window starts, or 0 if it is already active. Omit `bedtime_in` when lock is off. Kid plugin and parent plugin both use this. Read tokens allowed. Never send the PIN or its hash.

### `POST /v1/asks`

Ask token only. Queues a request. Does not credit time.

```json
{
  "group": "fun",
  "seconds": 900,
  "reason": "one more video"
}
```

### `GET /v1/asks`

Pending asks for the parent plugin.

### `POST /v1/asks/{id}/decide`

```json
{ "decision": "approve" }
```

Approve performs one grant. Source is `ask:<id>`. Group and seconds come from the ask. Outside bedtime it adds those seconds. During bedtime with lock on it sets remaining to those seconds and stay-up until they run out. Deny records the ask as denied. Either way the ask leaves pending. Approve does not clear parent lock.

A second decide on the same id returns 409. Body may say already decided. Do not grant twice.

### `POST /v1/pair`

No bearer. First LAN parent wins.

If the peer is not a private, loopback, link-local, or ULA address, 403. If a remote parent has already claimed this box, 409. The local bootstrap parent used by `setup kid` does not count as a claim.

Otherwise mint a parent token named `parent-pair` and return the secret once:

```json
{ "name": "kid-a", "id": "<machine-id>", "token": "<secret>" }
```

Grant, ask, lock, and the rest stay bearer-only.

### `POST /v1/tokens`

Parent token only. Mints a new key, returns the secret once, stores the hash.

```json
{
  "name": "khan-webhook",
  "kind": "app",
  "groups": ["fun"],
  "max_seconds_per_grant": 600,
  "max_seconds_per_day": 1800
}
```

`kind` is `parent`, `read`, `ask`, or `app`. Omit `groups` on `app` only when a group id `fun` exists. Then the daemon stores `["fun"]`. Otherwise the mint must name groups. `read` and `ask` ignore `groups`. `parent` may omit `groups` and mean all.

### `POST /v1/lock`

Parent token only.

```json
{ "locked": true }
```

200 `{"locked": true}`. Same body twice is 200. Ask, read, and app tokens get 403. Parent lock does not freeze a kid box until a parent PIN is set.

### `PUT /v1/parent-pin`

Parent token only. One household PIN, four digits.

```json
{ "pin": "1234" }
```

or the encoded argon2id string the parent CLI already hashed:

```json
{ "hash": "$argon2id$v=19$m=16384,t=1,p=1$..." }
```

200 `{"parent_pin_set": true}`. Ask, read, and app tokens get 403.

### `POST /v1/pin/approve`

Ask or read bearer plus `{ "pin": "1234", "ask_id": "..." }`. Verifies the PIN on the daemon, then approves that ask the same as parent decide (`source` `ask:<id>`). Wrong PIN is 403. Cooldown after too many misses is 403. Parent-desk decide still works during cooldown.

### `POST /v1/pin/grant`

Ask or read bearer plus `{ "pin": "1234", "seconds": 600 }`. Credits `fun` with `source` `parent-pin` and clears `parent_locked`. Outside bedtime it adds those seconds. During bedtime with lock on it sets remaining to those seconds (leftover daily minutes are discarded) and stay-up until they run out, so the bedtime overlay lifts. When remaining hits 0, the bedtime overlay returns. Midnight during stay-up does not refill remaining; that refill waits until stay-up ends. Hold expires at wake and does not stack leftover stay-up onto the new day's hours.

### `POST /v1/mode`

Parent token only.

```json
{ "id": "morning" }
```

Sets remaining to that mode's Look hours. Same id twice is 200 and **does** refill remaining. That is how Edit-then-tap-again works.

### `PATCH /v1/policy`

Parent token only. Partial JSON. Omitted fields stay. Empty string on a clock means keep.

```json
{
  "bedtime_start": "20:00",
  "bedtime_end": "07:00",
  "bedtime_lock": false,
  "modes": { "evening": { "fun": 1800 } }
}
```

Shim: writes bedtime and/or mode hours onto Look. Does not refill remaining, even if that mode is active. Do not set `bedtime_lock` true on the parent lab.

### `GET /v1/look`

Parent token only. Returns the Look document with `things`, `apps`, and `fun_hours`. It does not send `catalog` or `matchers`. Read, ask, and app tokens get 403. `GET /v1/search` is parent-only.

### `PUT /v1/look`

Parent token only. Validates the document, persists `meta.look`, bumps `look_version`, does not touch balances or `active_mode`. Same policy bytes is a no-op for remaining and version. Invalid overlap, exclusive-app, or bedtime errors are 400.

## Config

Root-owned `/etc/kidtimer/config.toml`. Two examples in `packaging/`.

Parent lab, the only config that may be installed on a parent workstation:

```toml
kid_name = "parent-lab"
timezone = "America/New_York"
enforcer = false
advertise = false
listen = "127.0.0.1:8742"
bedtime_lock = false
bedtime_start = "21:00"
bedtime_end = "07:00"
always_on = [
  "omarchy-shell",
  "omarchy.lock",
  "omarchy.menu",
  "omarchy.notifications",
  "omarchy.polkit",
]
never_signal = ["sshd", "tailscaled", "systemd", "Hyprland", "omarchy-shell"]
remote_lock = false

[lab]
enabled = true
classes = ["kidtimer-lab"]

[groups.school]
policy = "bypass"
match = [
  { class = "chrome-www.khanacademy.org__-Default" },
]

[groups.minecraft]
policy = "metered"
daily_seconds = 3600
overflow = "fun"
match = [
  { class = "(?i)minecraft|prismlauncher|org.prismlauncher" },
]

[groups.youtube]
policy = "metered"
daily_seconds = 3600
overflow = "fun"
match = [
  { class = "chrome-www.youtube.com__-Default" },
]

[groups.fun]
policy = "metered"
daily_seconds = 0
match = [
  { class = "kidtimer-lab" },
  { class = "(?i)steam|steamwebhelper" },
]

[modes.morning]
minecraft = 0
youtube = 0
fun = 0

[modes.homework]
minecraft = 0
youtube = 0
fun = 0

[modes.evening]
minecraft = 3600
youtube = 3600
fun = 0
```

The parent lab ships mode tables so parent lock and ApplyMode can be tested. It has no `[[schedule]]` windows. A weekday morning Status must not zero remaining on this workstation.

Kid box, not for the parent desk. Same groups. Fun does not include `kidtimer-lab`. `remote_lock = true` so parent lock freezes the kid overlay when a PIN is set.

```toml
kid_name = "kid-a"
timezone = "America/New_York"
enforcer = true
advertise = true
listen = "0.0.0.0:8742"
bedtime_lock = true
bedtime_start = "21:00"
bedtime_end = "07:00"
always_on = [
  "omarchy-shell",
  "omarchy.lock",
  "omarchy.menu",
  "omarchy.notifications",
  "omarchy.polkit",
]
never_signal = ["sshd", "tailscaled", "systemd", "Hyprland", "omarchy-shell"]
remote_lock = true

[lab]
enabled = false
classes = []

[groups.school]
policy = "bypass"
match = [
  { class = "chrome-www.khanacademy.org__-Default" },
]

[groups.minecraft]
policy = "metered"
daily_seconds = 3600
overflow = "fun"
match = [
  { class = "(?i)minecraft|prismlauncher|org.prismlauncher" },
]

[groups.youtube]
policy = "metered"
daily_seconds = 3600
overflow = "fun"
match = [
  { class = "chrome-www.youtube.com__-Default" },
]

[groups.fun]
policy = "metered"
daily_seconds = 0
match = [
  { class = "(?i)steam|steamwebhelper" },
]

[modes.morning]
minecraft = 0
youtube = 0
fun = 0

[modes.homework]
minecraft = 0
youtube = 0
fun = 0

[modes.evening]
minecraft = 3600
youtube = 3600
fun = 0

[[schedule]]
days = ["mon", "tue", "wed", "thu", "fri"]
start = "08:00"
end = "10:00"
mode = "morning"
```

On a kid box, unknown focused windows are ignored. On the parent desk, that is also true. Lab mode may freeze only `lab.classes`.

Parent household file, user-owned, written by `kidtimer parent` after pair:

```json
{
  "kids": [
    { "id": "<machine-id>", "name": "kid-a", "url": "http://192.168.1.20:8742", "token": "<parent-token>" }
  ]
}
```

Path is `~/.local/share/kidtimer/kids.json` mode `0600`. The household PIN hash is `~/.local/share/kidtimer/parent-pin` mode `0600`. The parent plugin watches both. Optional `settings.kids` in `shell.json` stays as a localhost lab pin. Auto-paired machines appear beside pins. They do not replace them. Pairing a new kid pushes the stored hash.

`kid_name` empty in TOML means the machine hostname.

## Plugins

Omarchy 4 plugins are `manifest.json` plus QML. Kinds: `bar-widget`, `panel`, `overlay`, `menu`, `service`, `bar`. A plugin can be several at once.

`omarchy plugin add <git-url>` clones a repo whose `manifest.json` is at the git root. This repo is a monorepo. `kidtimer setup parent` and `kidtimer setup kid` symlink the matching plugin into `~/.config/omarchy/plugins/`. Each of those directories has `manifest.json` at its root. Do not expect `omarchy plugin add` on the repo URL to work until the plugins are split. Directory listing later is `omarchyplugins.com`.

This repo ships two plugins, because parent and kid are different machines.

**`kidtimer.kid`**

- `bar-widget`: remaining time for the focused group, or bedtime
- `service` (`keepLoaded`): fullscreen overlay when status `overlay` is true. Exclusive keyboard. Super submap while shown. Stay-awake inhibit so Omarchy idle cannot real-lock on top. Fail open if the shell dies.
- On crossing 15, 5, and 1 minute of spend-path remaining for the focused metered group, fire an Omarchy notification. Clicking the toast opens the kid plugin. Same ladder for `bedtime_in` when bedtime lock is on. Seed on first poll and on first sight of a group. Do not fire at zero. Bypass and idle skip group warnings. A grant that raises remaining can cross the same ladder again.
- `panel`: request more time (−5 / +5, 5–120, default 30). While Waiting, Parent Pin approves that ask locally.
- Overlay Ask (−10 / +10, 10–120, default 30) then `POST /v1/asks`. Waiting while a pending ask exists. Overlay Parent Pin then asks how much more time (−5 / +5, 5–120, default 30) and `POST /v1/pin/grant`.
- Talks to `http://127.0.0.1:8742` with a read token plus an ask token
- Never writes the ledger itself

**`kidtimer.parent`**

- `bar-widget` or `panel`: one row per kid
- +10 / −10 apply a 600-second remaining delta to a chosen group, default `fun` when that id exists
- Pending asks, approve / deny
- Settings: Parent PIN row first, then Bed / Up, then weekday hours. Set invokes `kidtimer pin set`.
- Lock square does nothing until the household PIN exists
- Paired kids go through the parent desk on loopback. Optional `settings.kids` pins stay as a localhost lab path.
- Starts `~/.local/bin/kidtimer parent`, which browses `_kidtimer._tcp` and pairs. The widget restarts that process if it exits. It does not depend on `PATH`.
- Poll `GET /v1/asks` about every 5 seconds. Compare pending ids. On a new id, fire an Omarchy notification

Do not implement a Quickshell `service` that thinks it is the bank. A plugin `service` kind still runs as the user inside the shell. The bank is systemd. The kid overlay service is a screen, not the bank.

Prefer the Kidtimer overlay for bedtime, empty, and parent lock. Never call `omarchy system lock`. Escape does not dismiss the overlay. A real session lock is how you strand a machine.

## Daemon

Language: Go. One static binary named `kidtimer`. Subcommands:

```
kidtimer daemon          # systemd ExecStart
kidtimer setup parent    # user install on the parent desk
kidtimer setup kid       # root install on the kid box
kidtimer parent          # browse LAN, pair, write kids.json
kidtimer pin set         # household PIN; hash file plus PUT to kids
kidtimer pin status      # whether the household PIN file exists
kidtimer token create    # HTTP POST /v1/tokens, print secret once
kidtimer grant           # HTTP POST /v1/grants
kidtimer status          # HTTP GET /v1/status
kidtimer lock            # HTTP POST /v1/lock
kidtimer unlock          # HTTP POST /v1/lock
kidtimer asks            # HTTP GET /v1/asks
kidtimer decide          # HTTP POST /v1/asks/{id}/decide
```

`lock`, `unlock`, `grant`, `status`, `asks`, and `decide` are HTTP clients of the parent desk on loopback, or of a kid daemon when both a URL and a token are set. They do not open sqlite.

State:

- `/var/lib/kidtimer/ledger.sqlite`
- `/etc/kidtimer/tokens.json` hashes only
- append-only rows in sqlite, not a log file the kid can truncate without sudo

systemd unit `kidtimer.service`, system, restart on failure, after `tailscaled.service`.

The unit runs as root so the kid cannot truncate the ledger without sudo. Hyprland is a session-user API. Each tick, resolve the kid uid, set `XDG_RUNTIME_DIR=/run/user/<uid>`, pick the Hyprland instance under `/run/user/<uid>/hypr/`, and run `hyprctl` with that env. Do not run `omarchy system lock`. If Hyprland is down, skip the sample. Do not overlay the parent desk. Do not add a second session-helper daemon.

Enforcer loop, only when `enforcer = true`: follow Match and enforce. Lab mode on the parent desk restricts which classes it may freeze. `bedtime_lock = false` skips the bedtime freeze. `remote_lock = false` skips freeze for parent lock. No parent PIN skips freeze entirely.

Tests must not call live `hyprctl` on the parent desk. The enforcer takes a focus source interface. Production uses Hyprland. Tests inject fake windows and a fake process table, or SIGSTOP a dedicated `sleep` child started by the test, never sshd. Stage C must assert `SIGCONT` after a grant, not only pause at zero.

## Repo layout

```
omarchy-kidtimer/
  spec.md                 # this file
  daemon/                 # Go module
  plugin-kid/             # manifest.json + QML, id kidtimer.kid
  plugin-parent/          # manifest.json + QML, id kidtimer.parent
  packaging/              # systemd unit, example config.toml
  testdata/               # curl scripts for the API
```

Do not require the daemon to live inside the plugin directory. The daemon is not QML.

## Test plan

Treat the parent desk as hostile to freeze and lock. A parent may be SSH-only.

Do not SIGSTOP Cursor, a terminal, sshd, Tailscale, or the daily browser on the parent desk. Do not call `omarchy system lock` on the parent desk. Do not show the Kidtimer overlay on the parent desk.

The bank is done when `go test ./...` is green. Plugin UI starts after that, not before.

**Stage A. Ledger, no HTTP, no systemd.** `go test` on the ledger package. Grant, cap, midnight reset, pile spend with no overflow, bypass never decrements, no-match ignores, idempotency, token scope, app token cannot skip bedtime, ask does not credit until decide, parent lock persist, PUT look and PATCH policy leave remaining. Safe over SSH. No Hyprland.

**Stage B. HTTP API.** Daemon or `httptest` on localhost, `enforcer = false`, `bedtime_lock = false`. `go test` plus curl in `testdata/`. Covers `POST /v1/grants`, status (`look_version`, `focused_app`), GET/PUT `/v1/look`, asks, decide, token mint, lock, bad bearer, replayed idempotency key, same key different body → 409, second decide → 409, leftover `POST /v1/mode` must not change remaining, PATCH policy does not refill. Safe over SSH.

**Stage C. Enforcer with a fake compositor.** Inject "Minecraft class focused for 10s" and assert remaining dropped, then a pause action recorded against a test PID. Grant time. Assert `SIGCONT` on that PID. Optionally spawn `sleep 3600` as a child of the test and signal that PID only. Never poll live `hyprctl`. Never lock the session. Safe over SSH. This is the gate that says pause, resume, and bedtime logic work.

**Stage D. Plugin UI.** QML against the local Stage B daemon. Bar countdown, +10 / −10 to a chosen group, ask, approve. No new enforcement here. The plugin is a client of an API the tests already proved.

**Stage E. Live lab window, at a desk.** `enforcer = true`, `lab.enabled = true`, class `kidtimer-lab`, `bedtime_lock = false`. Skip while SSH-only. Kill switch is `sudo systemctl stop kidtimer`.

**Stage F. Kid machine, person at that desk.** `sudo kidtimer setup kid` on a second Omarchy box. Parent tape on the parent Omarchy 4 desk shows that hostname without editing `shell.json`. Confirm `--app=` window classes with `hyprctl activewindow -j` if Chromium used a different app id. Then `kid-b` for Minecraft.

Do not develop by logging out of the parent desk into a second graphical user. Do not wipe Windows to start.

## Out of scope for v1

- HomeschoolOS glue
- Per-tab Chrome
- Cloud control plane or a Pi hub
- Funnel / public webhook
- Kid UI to allocate minutes across games
- BIOS, live USB, TTY lockdown, no-sudo hardening
- Donut charts, weekly trends, competing with the tracker plugin
- Counting Windows dual-boot time
- Kids hopping machines and taking a balance with them
- Raising caps or skipping bedtime from an app token
- Default-deny / kiosk of unknown windows
- Refuse launch
- Overflow. Spend the matched pile only.
- Daily ceiling across groups
- Parent QML Look editor in this unit. GET/PUT `/v1/look` is the daemon contract. Schedule calendar UI lives in the parent-look plugin work.

## Implementer start

1. Stage A ledger tests, green.
2. Stage B HTTP, green. Daemon on the parent desk is fine. Enforcer off.
3. Stage C fake-compositor enforcer tests, green.
4. Stop. Do not start QML until A through C pass.
5. Stage D plugins against localhost.
6. Stage E only with someone looking at the screen.
7. Stage F on `kid-a` at that desk.
