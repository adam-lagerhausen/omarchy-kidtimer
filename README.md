# Kidtimer

Kidtimer puts a daily time limit and a bedtime on a kid's Omarchy computer, with a control panel on yours. When time runs out, bedtime begins, or you hit Lock from your desk, a full-screen overlay covers their session. Super and Escape will not get them out.

This is unofficial and not affiliated with Omarchy.

```
              +----------------+
              |     parent     |
              |  kid 1 · kid 2 |
              +--------+-------+
                       |
                  home Wi-Fi
                       |
          +------------+------------+
          |                         |
   +------+-----+            +------+-----+
   |   kid 1    |            |   kid 2    |
   | 47min left |            |   1h left  |
   +------------+            +------------+
```

![Parent panel](docs/screenshots/parent.png)

## Install

You need two Omarchy 4 computers on the same home network. A coffee-shop or open guest network is not enough; see [Limits](#limits).

Do the kid computer first, then yours, so your desk can claim it as soon as it comes on the network — no pairing codes to copy.

1. Open a terminal with Super + Return (Windows key plus Enter).
2. Paste this and press Enter:

```
curl -fsSL https://raw.githubusercontent.com/adam-lagerhausen/omarchy-kidtimer/master/install.sh | bash
```

The installer checks a checksum against the latest release. You can open that URL and read the script before running it.

Do not run `omarchy plugin add` on this repo URL — that path does not work.

3. When it asks, pick Mine or The kid's.
4. On the kid's computer it may ask for your password.

The kid computer shows up on your bar as its hostname. Rename the machine first if you want a name like Ada. If the bar does not change, run `omarchy restart shell`.

Left-click the Kidtimer chip to open the panel. You should see their computer and the minutes left. Right-click the chip to grant +10.

Set a 4-digit PIN next. Until one exists, time still counts but nothing covers their screen, and the Lock square does nothing.

To update later, run the same install command again. It pulls the latest release.

## How it works

The panel shows time left, +10 and −10, Lock, and pending asks. Hours and bedtime live behind the gear. When they ask for more time, you get an Omarchy notification with their name; approve or deny from the panel.

![Settings](docs/screenshots/settings.png)

Games and apps spend the hour. The bar, the launcher, idle time, and the session lock do not.

At midnight the clock refills from that day's hours. Extra time from yesterday does not stack.

### Defaults

The clock is already running: 1 hour Monday through Friday, 2 hours Saturday and Sunday, bedtime 9:00 PM to 7:00 AM. Those times follow the timezone in `/etc/kidtimer/config.toml` on the kid box. The installer leaves that as America/New_York, so change it if that is not your house.

### When time runs out

Time at zero, bedtime, and Lock from your desk all raise that overlay. It says 0m LEFT, bedtime, or locked.

![Time's up](docs/screenshots/overlay.png)

![Bedtime](docs/screenshots/bedtime.png)

It sits over a still-running session — not the Omarchy lock screen, and not the login screen. Once it is up, Ask is gone from that screen. Add time from your desk, or type the parent PIN on theirs.

### Asking for more

They tap Ask on their bar and pick minutes from 5 to 120. They cannot ask during bedtime or a parent lock.

![Pending asks](docs/screenshots/ask.png)

The parent PIN on the overlay adds minutes and clears a parent lock. During bedtime it also grants stay-up until morning, so the bedtime cover lifts. If those minutes hit zero before then, the 0m LEFT overlay comes back. The PIN does not change the scheduled hours or bedtime.

Five wrong guesses start a 30-second cooldown.

### If you are not home

The kid computer keeps counting, and the overlay still works. Your desk needs the home network to see them, approve asks, grant +10 or −10, and lock.

## Limits

This is not a kiosk.

A kid who knows the computer password can stop it:

```
sudo systemctl stop kidtimer
```

If they kill the shell, the cover is gone and Super works again. A TTY, a reboot, or Windows on a dual-boot disk all get them out. Time only counts while Omarchy is running.

Only one parent computer. The first desk that sees a new kid computer claims it; later desks cannot. Do that on a quiet home network with your desk awake, and keep guest laptops off Kidtimer until yours has claimed the box and the PIN is set.

The kid computer talks on your private network with no encryption, on port 8742. Tailscale at home counts. Do not do this on cafe Wi-Fi or an open guest network.

## Take it off

On the kid computer:

```
sudo systemctl disable --now kidtimer
rm -rf ~/.config/omarchy/plugins/kidtimer.kid
```

On yours:

```
rm -rf ~/.config/omarchy/plugins/kidtimer.parent
```

Then `omarchy restart shell` on both.

If the chip stays, take the kidtimer line out of `~/.config/omarchy/shell.json` and restart the shell.

Leftovers on the kid box: `/etc/kidtimer` and `/var/lib/kidtimer`. On yours: `~/.local/share/kidtimer`.

## From source

You need Go 1.25.

```
git clone https://github.com/adam-lagerhausen/omarchy-kidtimer.git
cd omarchy-kidtimer
go test ./...
go build -o kidtimer ./daemon/cmd/kidtimer
```

`go test ./...` from the repo root must print PASS.

Then `./kidtimer setup parent` on the parent desk, or `sudo ./kidtimer setup kid` on the kid box, from that checkout.

`packaging/pack.sh` writes the tarballs. `packaging/install.sh` is the unpack installer.

## License

The daemon is MIT; the text is in `LICENSE`. IBM Plex Mono is OFL, not MIT; that text is in `plugin-parent/fonts/OFL.txt`.

Report a lock bypass privately; see `SECURITY.md`.
