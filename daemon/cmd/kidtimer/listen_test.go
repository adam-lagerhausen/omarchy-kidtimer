package main

import (
	"testing"

	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/netaddr"
)

func TestParentLabListenStaysLocalhost(t *testing.T) {
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Advertise {
		t.Fatal("parent-lab advertise")
	}
	spec := listenSpec(cfg, "127.0.0.1:9123")
	if spec != "127.0.0.1:9123" {
		t.Fatalf("flag wins: %s", spec)
	}
	if listenSpec(cfg, "") != "127.0.0.1:8742" && listenSpec(cfg, "") != cfg.Listen {
		t.Fatalf("parent-lab listen %q", listenSpec(cfg, ""))
	}
	addrs, err := netaddr.ParseListen(listenSpec(cfg, ""))
	if err != nil || len(addrs) != 1 || addrs[0] != "127.0.0.1:8742" {
		t.Fatalf("parent-lab listen: %v %v", addrs, err)
	}
}

func TestKidListenIsExplicit(t *testing.T) {
	cfg, err := config.ParseFile(packagingPath(t, "config.kid.toml"))
	if err != nil {
		t.Fatal(err)
	}
	addrs, err := netaddr.ParseListen(listenSpec(cfg, ""))
	if err != nil || len(addrs) != 1 || addrs[0] != "127.0.0.1:8742" {
		t.Fatalf("kid listen is loopback: %v %v", addrs, err)
	}
}

func TestAdvertiseWithoutListenStaysLoopback(t *testing.T) {
	cfg := &config.Config{Advertise: true}
	if listenSpec(cfg, "") != "127.0.0.1:8742" {
		t.Fatalf("advertise default listen %q", listenSpec(cfg, ""))
	}
	if listenSpec(cfg, "127.0.0.1:9123") != "127.0.0.1:9123" {
		t.Fatal("flag still wins")
	}
}

func TestOpenListenersKeepsGoing(t *testing.T) {
	lns, err := openListeners([]string{"[fe80::1]:1", "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(lns) == 0 {
		t.Fatal("expected a live localhost listener")
	}
	for _, ln := range lns {
		_ = ln.Close()
	}
}
