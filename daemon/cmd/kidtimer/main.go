package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"kidtimer/daemon/internal/advertise"
	"kidtimer/daemon/internal/bank"
	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/enforcer"
	"kidtimer/daemon/internal/httpapi"
	"kidtimer/daemon/internal/look"
	"kidtimer/daemon/internal/netaddr"
	"kidtimer/daemon/internal/session"
)

func main() {
	if len(os.Args) < 2 {
		fatal("usage: kidtimer daemon|setup|parent|kid|grant|status|token|pin|lock|unlock|asks|decide")
	}
	switch os.Args[1] {
	case "daemon":
		fatalErr(runDaemon(os.Args[2:]))
	case "setup":
		fatalErr(runSetup(os.Args[2:]))
	case "parent":
		fatalErr(runParent(os.Args[2:]))
	case "kid":
		fatalErr(runKid(os.Args[2:]))
	case "lock", "unlock", "grant", "status", "asks", "decide":
		req, err := Parse(os.Args[1], os.Args[2:])
		fatalErr(err)
		fatalErr(Do(req))
	case "token":
		if len(os.Args) < 3 || os.Args[2] != "create" {
			fatal("usage: kidtimer token create")
		}
		fatalErr(runTokenCreate(os.Args[3:]))
	case "pin":
		fatalErr(runPin(os.Args[2:]))
	default:
		fatal("usage: kidtimer daemon|setup|parent|kid|grant|status|token|pin|lock|unlock|asks|decide")
	}
}

func runDaemon(args []string) error {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	cfgPath := fs.String("config", "/etc/kidtimer/config.toml", "config path")
	dbPath := fs.String("data", "/var/lib/kidtimer/ledger.sqlite", "sqlite path")
	listen := fs.String("listen", "", "HTTP listen address (comma-separated); default 127.0.0.1:8742")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := config.ParseFile(*cfgPath)
	if err != nil {
		return err
	}
	b, err := bank.Open(*dbPath, cfg, time.Now)
	if err != nil {
		return err
	}
	defer b.Close()
	n, err := b.TokenCount()
	if err != nil {
		return err
	}
	if n == 0 {
		secret, tok, err := b.SeedParent("bootstrap")
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "bootstrap parent token %s (shown once): %s\n", tok.Name, secret)
	}

	api := httpapi.New(b)
	hypr := &enforcer.Hyprland{}
	logind := &enforcer.LogindSession{Watch: &enforcer.InputIdle{}}
	attachSession := func() int {
		uid, err := session.GraphicalUID()
		if err != nil || uid <= 0 {
			uid = 0
		}
		hypr.UID = uid
		logind.UID = uid
		return uid
	}
	attachSession()
	api.SearchEnv = func() ([]look.InstalledApp, look.Focus, func(string) bool) {
		uid := attachSession()
		if uid <= 0 {
			return nil, look.Focus{}, cfg.AlwaysOnClass
		}
		home, err := session.Home(uid)
		if err != nil {
			return nil, look.Focus{}, cfg.AlwaysOnClass
		}
		apps, err := look.ScanDesktops(look.DesktopDirs(home))
		if err != nil {
			apps = nil
		}
		focus := look.Focus{}
		if w, ok, err := hypr.Active(); err == nil && ok {
			focus = look.Focus{Class: w.Class, Title: w.Title}
		}
		return apps, focus, cfg.AlwaysOnClass
	}
	var loop *enforcer.Enforcer
	if cfg.Enforcer {
		loop = &enforcer.Enforcer{
			Bank:       b,
			Focus:      hypr,
			Session:    logind,
			Signals:    enforcer.UnixSignaler{},
			SessionUID: attachSession,
		}
		api.Resume = func(group string) {
			_ = loop.Resume(group)
		}
	}

	if cfg.Advertise {
		id, err := advertise.MachineID(filepath.Join(filepath.Dir(*dbPath), "machine-id"))
		if err != nil {
			return err
		}
		api.MachineID = id
		port, err := advertise.PortOf(firstListen(listenSpec(cfg, *listen)))
		if err != nil {
			return err
		}
		svc, err := advertise.Register(cfg.KidName, id, port)
		if err != nil {
			fmt.Fprintf(os.Stderr, "mdns advertise: %v\n", err)
		} else {
			defer svc.Shutdown()
		}
	}

	addrs, err := netaddr.ParseListen(listenSpec(cfg, *listen))
	if err != nil {
		return err
	}
	lns, err := openListeners(addrs)
	if err != nil {
		return err
	}
	releasePID := claimUserBankPID(*dbPath)
	defer releasePID()
	srv := &http.Server{Handler: api.Handler()}
	errCh := make(chan error, 1)
	for _, ln := range lns {
		go func(ln net.Listener) { errCh <- srv.Serve(ln) }(ln)
	}

	if loop != nil {
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for range t.C {
				if err := loop.Tick(); err != nil {
					fmt.Fprintf(os.Stderr, "enforcer: %v\n", err)
				}
			}
		}()
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-ch:
		_ = srv.Close()
		fmt.Fprintf(os.Stderr, "stopped on %s\n", sig)
		return nil
	case err := <-errCh:
		return err
	}
}

func listenSpec(cfg *config.Config, flag string) string {
	if flag != "" {
		return flag
	}
	if cfg != nil && cfg.Listen != "" {
		return cfg.Listen
	}
	if cfg != nil && cfg.Advertise {
		return "0.0.0.0:8742"
	}
	return netaddr.DefaultListen
}

func firstListen(spec string) string {
	addrs, err := netaddr.ParseListen(spec)
	if err != nil || len(addrs) == 0 {
		return netaddr.DefaultListen
	}
	return addrs[0]
}

func openListeners(addrs []string) ([]net.Listener, error) {
	var lns []net.Listener
	var last error
	for _, addr := range addrs {
		ln, err := netaddr.TryListen(addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "listen %s: %v\n", addr, err)
			last = err
			continue
		}
		lns = append(lns, ln)
	}
	if len(lns) == 0 {
		if last != nil {
			return nil, last
		}
		return nil, fmt.Errorf("no listen address")
	}
	return lns, nil
}

func runTokenCreate(args []string) error {
	fs := flag.NewFlagSet("token create", flag.ContinueOnError)
	base := fs.String("url", envOr("KIDTIMER_URL", "http://127.0.0.1:8742"), "daemon URL")
	token := fs.String("token", os.Getenv("KIDTIMER_TOKEN"), "parent bearer token")
	name := fs.String("name", "", "token name")
	kind := fs.String("kind", "app", "parent, read, ask, or app")
	maxGrant := fs.Int("max-seconds-per-grant", 0, "cap per grant")
	maxDay := fs.Int("max-seconds-per-day", 0, "cap per day")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *name == "" {
		return fmt.Errorf("name is required")
	}
	body, _ := json.Marshal(map[string]any{
		"name": *name, "kind": *kind,
		"max_seconds_per_grant": *maxGrant,
		"max_seconds_per_day":   *maxDay,
	})
	return dump(post(*base+"/v1/tokens", *token, body, ""))
}

func post(url, token string, body []byte, idem string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	return http.DefaultClient.Do(req)
}

func dump(resp *http.Response, err error) error {
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return copyResp(resp)
}

func copyResp(resp *http.Response) error {
	if _, err := io.Copy(os.Stdout, resp.Body); err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(2)
}

func fatalErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
