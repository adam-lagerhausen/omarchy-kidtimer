package dial

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"kidtimer/daemon/internal/bank"
	"kidtimer/daemon/internal/httpapi"
	"kidtimer/daemon/internal/netaddr"
	"kidtimer/daemon/internal/reverse"
)

var (
	systemConfigPath = "/etc/kidtimer/config.toml"
	bankWait         = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 15 * time.Second}
)

func takeBank(ctx context.Context, cfg *Config) (func(), error) {
	if cfg.Bank != nil {
		return func() {}, nil
	}
	addr := cfg.BankHTTP
	if addr == "" {
		addr = DefaultBankHTTP
	}
	cfg.BankHTTP = addr
	if bankUp(addr) {
		return func() {}, nil
	}
	if systemKidInstalled() {
		return waitForSystemBank(ctx, addr)
	}
	return startPickupBank(cfg, addr)
}

func systemKidInstalled() bool {
	if systemConfigPath == "" {
		return false
	}
	_, err := os.Stat(systemConfigPath)
	return err == nil
}

func waitForSystemBank(ctx context.Context, addr string) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	step := 0
	for {
		if bankUp(addr) {
			return func() {}, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("dial: waiting for system bank at %s: %w", addr, err)
		}
		d := bankWait[0]
		if step < len(bankWait) {
			d = bankWait[step]
			if step < len(bankWait)-1 {
				step++
			}
		}
		timer := time.NewTimer(d)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("dial: waiting for system bank at %s: %w", addr, ctx.Err())
		case <-timer.C:
		}
	}
}

func startPickupBank(cfg *Config, addr string) (func(), error) {
	ln, err := netaddr.TryListen(addr)
	if err != nil {
		if bankUp(addr) {
			return func() {}, nil
		}
		return nil, fmt.Errorf("dial: bank listen %s: %w", addr, err)
	}
	name := cfg.Name
	if name == "" {
		host, _ := os.Hostname()
		name = reverse.KidName(host)
	}
	pcfg, err := PickupConfig(name)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	if err := os.MkdirAll(cfg.Home, 0o700); err != nil {
		_ = ln.Close()
		return nil, err
	}
	b, err := bank.Open(filepath.Join(cfg.Home, "ledger.sqlite"), pcfg, nil)
	if err != nil {
		_ = ln.Close()
		return nil, err
	}
	n, err := b.TokenCount()
	if err != nil {
		_ = b.Close()
		_ = ln.Close()
		return nil, err
	}
	if n == 0 {
		if _, _, err := b.SeedParent("local"); err != nil {
			_ = b.Close()
			_ = ln.Close()
			return nil, err
		}
	}
	srv := &http.Server{Handler: httpapi.New(b).Handler()}
	go func() { _ = srv.Serve(ln) }()
	cfg.Bank = b
	return func() {
		_ = srv.Close()
		_ = b.Close()
	}, nil
}

func bankUp(addr string) bool {
	c := &http.Client{Timeout: 400 * time.Millisecond}
	resp, err := c.Get("http://" + addr + "/v1/status")
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusOK
}
