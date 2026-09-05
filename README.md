# Kidtimer

Kidtimer enforces screen-time limits on an Omarchy 4 kid box. The parent desk runs a control panel. Apps grant time through the grant API. This is not a screen-time tracker.

This project is unofficial. Kidtimer is not affiliated with Omarchy.

## Install

Unpack the pack on each machine, then run one command. Do not guess parent vs kid.

On the parent Omarchy 4 desk:

```
./install.sh parent
```

That command copies the parent plugin into `~/.local/share/kidtimer/src` and installs `~/.local/bin/kidtimer`.

The kid daemon binds 0.0.0.0 port 8742 with no TLS. Run the next command only on a trusted LAN.

On the kid Omarchy 4 box:

```
sudo ./install.sh kid
```

That command copies the kid plugin into `/usr/local/share/kidtimer`. It installs `/usr/local/bin/kidtimer`. It writes kid config. Then it enables the systemd unit.

If the bar does not update: `omarchy restart shell`.

Do not run `omarchy plugin add` on this repo URL. That path does not work. The install script already symlinks the matching plugin.

A kid with sudo can stop the unit.

Stolen app token equals that token's daily cap.

## Pair on the LAN

Same LAN is enough. The kid advertises. The parent pairs. Do not paste a token.

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

`packaging/pack.sh` writes `dist/kidtimer-linux-amd64.tar.gz` and `dist/kidtimer-linux-arm64.tar.gz`. Unpack one of those and run `./install.sh parent` or `sudo ./install.sh kid`.

## License

The daemon is MIT. The MIT text is in `LICENSE`.

IBM Plex Mono is OFL, not MIT. The OFL text is in `plugin-parent/fonts/OFL.txt`.
