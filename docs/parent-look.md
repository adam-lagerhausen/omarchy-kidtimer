# Parent panel look

Locked 3 Sep 2026. One daily timer. Continue from here. Do not go back to Fun vs School lists.

This document specifies the look. Prototypes are not in the public tree.

This is what the parent sees. You pick storage, HTTP, and QML. Layout matches this document. Color follows the Omarchy theme.

Gear flips the tape to settings. Same panel. Not a second window, not a console.

## Who this is for

A parent on the parent Omarchy 4 desk. They are looking at two kids' computers. They are tired. They want to lock a session, add ten minutes, see what the kids already did today, and clear asks without hunting. Gear is where they set the household parent PIN, bedtime, and how much time each day. If a label needs a footnote, it is wrong.

## The box

The panel sits on the Omarchy popup surface. Type is popup or bar foreground. Alarms are urgent. Secondary is muted. The live dot is accent. JetBrains Mono only. About 340px wide. Square 1px controls. No pills, no glass, no perforated top and bottom, no extra title. The host popup border follows the theme.

It hangs off the Omarchy bar. Home is the front of the tape. Gear flips it.

Order on home:

1. Bell, kid dropdown, lock square
2. Asks, only if the bell is open
3. Used minutes
4. Minutes left with −10 / +10, remaining bar
5. 24 hour track of what they did
6. Session log
7. Settings gear, bottom right

No mode buttons. No Free time / School time / Fun time keys. No Fun or School lists. No `+ new mode`. No `edit groups`. No `edit schedule`.

## Bell

Square, bell icon. Household inbox. The count is every pending ask from every kid. Switching computers does not change the badge.

Asks hide until someone hits the bell. Then every pending ask is a card: who asked, what they asked for, Deny, Approve. Approve credits the minutes they asked for, on that kid, not whoever is selected. Deny drops it. Empty bell does nothing flashy.

## Settings

Square, gear, bottom right. It flips the tape. Gear again brings home back. The gear fills while you are on it.

Still the selected kid. The dropdown still switches computers. Lock is not on this page. Bell still opens asks. An ask is the same card as home.

Order on the settings tape:

1. Bell, kid dropdown
2. Asks, only if the bell is open
3. Parent PIN. Four boxes and Set. This is the household 4-digit PIN, not the computer password. Lock does nothing until it is set.
4. CLOCK 12 or 24. Filled square is the active choice. Bed, the day log, the track marks, and the kid overlay follow it.
5. Bed and Up as two clocks. −  9:00 PM  + on one row, −  7:00 AM  + on the next. The hatched night under them.
6. Time for each day of the week, each with −15 / +15
7. Filled gear, bottom right

Bedtime is two clocks, not two more day rows. `9:00 PM` is bed, `7:00 AM` is up. − and + move 15 minutes. The hatch under them is the night: same marks as home, no sessions, no needle. Home's hatch uses those times.

Each day has its own number. Sample: Monday through Friday one hour, Saturday and Sunday two. Midnight fills that day's number. `+10` on home is for today. It does not change the day's line.

## Computers

The dropdown is a real control: border, chevron, name, what they are on. Click it for every computer. Two kids or five, same control.

A circle sits to the left of the name. Accent means they are on the computer. Urgent means they are off it. Quieter type to the left of the name always has a status: `Active`, `Active · minecraft`, `Offline`, `Error`, `Locked`, `Bedtime`, or `Already claimed`. Error wins over everything. Offline wins over lock and bedtime. Locked wins over bedtime. If it is locked, the dropdown and the lock go urgent. Error is urgent too. Never `down`, never blank, never `on minecraft`.

Picking a computer you already own aims remaining, lock, the day log, and settings at that kid. It does not close the bell and it does not hide other kids' asks.

A computer another parent already claimed stays in the list. Clicking it asks to adopt: “Take over NAME? Another parent already claimed this computer. Yes makes it yours.” Yes takeovers. No closes the prompt only. The row stays. There is no blacklist. +10, lock, and asks stay off until Yes.

“No computers found” / “Install Kidtimer on the kid computer” only when scan found nobody. Claimed-only still opens the dropdown.

Do not show `reachable: yes`.

## Lock

Square next to the dropdown, on the same row as the bell. Open padlock unlocked, closed padlock locked. It locks or unlocks the selected kid. Remaining looks faded. A LOCKED stamp sits on the panel. The kid can Ask to unlock. Approve lifts the lock. No Seize, Release, ARM, session seized, hack the box.

The square does nothing until a Parent PIN exists (household file or any kid `parent_pin_set`). Clicking it then opens settings so the parent can Set the PIN. After that, lock is a flag on the kid. The kid box shows an overlay, not a real session lock.

## Time

One line. `USED` on the left, minutes used on the right.

Under that, a full-width row: −10 far left, minutes left centered (`16m LEFT`), +10 far right.

Under that, a remaining bar. Fill is minutes left against today's allotment. It shrinks as they spend. +10 grows it. At zero the number and the fill go urgent, and −10 does nothing.

## Today

The 24 hour track is what they already did, not a schedule to edit. Hatched night is bedtime. Accent marks are time on the computer. The needle is now.

The log under the track is sittings, not every window. A sitting is one stretch at the computer. Split when they leave for more than ten minutes. One row: when it started, what they were on, how long they sat. `foot` is Terminal. Two apps that both lasted two minutes or more share a row: `MINECRAFT + CHROME`. At most six rows. If they sat down more than that, the oldest fold into `EARLIER` and the five newest stay. `kidtimer parent export` prints every window.

`7:40 AM  CHROME      30m`
`3:58 PM  MINECRAFT   44m`

Bedtime locks the computer. It is not a mode. Settings is where the two clocks live. No chips for 8:30, 9:00, 9:30, 10:00.

## Words

Sound like a parent.

- `Lock Ada` / `Unlock Ada`
- `Active` / `Active · minecraft`
- `Offline` / `Error` / `Locked` / `Bedtime`
- `16m LEFT`
- `BED` / `UP`
- `CLOCK` / `12` / `24`
- `9:00 PM` / `7:00 AM`
- `MON` … `SUN`
- `Ada asked for 10 more minutes`
- `Ada asked to unlock`
- `DENY` / `APPROVE`

Do not sound like a sysadmin or a hacker movie. Never say overflow, pile, metered, bypass, or regex. Minecraft is Minecraft. Do not say Fun or School.

## Do not

- Mode buttons
- User-created modes
- Edit schedule, edit groups
- Fun or School lists
- A remaining number for School
- Asks that follow the selected kid
- TODAY as a heading
- Perforated receipt edges
- `type=time` fields for bedtime
- Time chips for 8:30, 9:00, 9:30, 10:00
- A form that needs a tour
- Tabs, Save, or a settings console
- Regex, class patterns, or group ids in the panel
