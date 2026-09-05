# Kidtimer

Screen time limits for an Omarchy kid computer. You get a panel on your desk.

This project is unofficial. Kidtimer is not affiliated with Omarchy.

## Install

Do the kid's computer first, then yours. Same Wi-Fi.

1. Open a terminal. Super + Enter.
2. Paste this. Press Enter.

```
curl -fsSL https://raw.githubusercontent.com/adam-lagerhausen/omarchy-kidtimer/master/install.sh | bash
```

3. When it asks, pick Mine or The kid's.
4. On the kid's computer it may ask for your password.

The kid's computer shows up on your bar. No codes to copy.

If the bar does not change: `omarchy restart shell`.

## Notes

The kid computer listens on the LAN, port 8742, with no TLS. Only do this at home.

A kid who knows the computer password can turn it off.

If an app token is stolen, that token's daily cap is all they get.

Do not run `omarchy plugin add` on this repo URL. That path does not work.

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

The daemon is MIT. The MIT text is in `LICENSE`.

IBM Plex Mono is OFL, not MIT. The OFL text is in `plugin-parent/fonts/OFL.txt`.
