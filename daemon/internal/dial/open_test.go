package dial

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kidtimer/daemon/internal/netaddr"
)

func TestTakeBankSystemInstallDoesNotListen(t *testing.T) {
	restoreSystem(t, mustFile(t, t.TempDir(), "config.toml"))
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := &Config{Home: t.TempDir(), BankHTTP: addr}
	stop, err := takeBank(ctx, cfg)
	if err == nil {
		if stop != nil {
			stop()
		}
		t.Fatal("system install must wait, not start a pickup bank")
	}
	if cfg.Bank != nil {
		t.Fatal("system install must not open a pickup bank")
	}
	ln, err := netaddr.TryListen(addr)
	if err != nil {
		t.Fatalf("port should be free: %v", err)
	}
	_ = ln.Close()
}

func TestTakeBankReusesExisting(t *testing.T) {
	restoreSystem(t, filepath.Join(t.TempDir(), "missing.toml"))
	h := http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	go func() { _ = h.Serve(ln) }()
	t.Cleanup(func() { _ = h.Close() })
	if !bankUp(addr) {
		t.Fatal("stub bank should be up")
	}
	cfg := &Config{Home: t.TempDir(), BankHTTP: addr}
	stop, err := takeBank(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	if cfg.Bank != nil {
		t.Fatal("existing bank must not start a pickup bank")
	}
	if !bankUp(addr) {
		t.Fatal("existing bank should stay up")
	}
}

func TestTakeBankPickupWhenNoSystemInstall(t *testing.T) {
	restoreSystem(t, filepath.Join(t.TempDir(), "missing.toml"))
	addr := freeAddr(t)
	cfg := &Config{Home: t.TempDir(), BankHTTP: addr, Name: "pickup"}
	stop, err := takeBank(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	if cfg.Bank == nil {
		t.Fatal("pickup should open a bank")
	}
	if !bankUp(addr) {
		t.Fatal("pickup should serve /v1/status")
	}
}

func TestTakeBankSystemInstallWaitsThenUsesBank(t *testing.T) {
	restoreSystem(t, mustFile(t, t.TempDir(), "config.toml"))
	bankWait = []time.Duration{5 * time.Millisecond}
	t.Cleanup(func() {
		bankWait = []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 15 * time.Second}
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	cfg := &Config{Home: t.TempDir(), BankHTTP: addr}
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var stop func()
	go func() {
		var err error
		stop, err = takeBank(ctx, cfg)
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	h := http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})}
	up, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = h.Serve(up) }()
	t.Cleanup(func() { _ = h.Close() })
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for system bank")
	}
	if stop != nil {
		t.Cleanup(stop)
	}
	if cfg.Bank != nil {
		t.Fatal("system install must not open a pickup bank")
	}
}

func restoreSystem(t *testing.T, path string) {
	t.Helper()
	prev := systemConfigPath
	systemConfigPath = path
	t.Cleanup(func() { systemConfigPath = prev })
}

func mustFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("kid_name = \"test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}
