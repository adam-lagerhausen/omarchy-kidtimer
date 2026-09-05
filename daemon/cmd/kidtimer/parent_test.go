package main

import (
	"os"
	"path/filepath"
	"testing"

	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

func TestEnsureHouseholdCreatesKidsFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "kidtimer")
	path, err := ensureHousehold(dir)
	if err != nil || path != household.Path(dir) {
		t.Fatalf("%s %v", path, err)
	}
	kids, err := household.Load(path)
	if err != nil || len(kids) != 0 {
		t.Fatalf("empty %v %v", kids, err)
	}
	if err := household.Save(path, []reverse.Record{{ID: "m1", Name: "sam", TicketHash: "h"}}); err != nil {
		t.Fatal(err)
	}
	again, err := ensureHousehold(dir)
	if err != nil || again != path {
		t.Fatalf("keep %s %v", again, err)
	}
	got, err := household.Load(path)
	if err != nil || len(got) != 1 || got[0].ID != "m1" {
		t.Fatalf("kept %v %v", got, err)
	}
}

func TestLockParentExclusive(t *testing.T) {
	dir := t.TempDir()
	a, err := lockParent(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if _, err := os.Stat(filepath.Join(dir, "parent.lock")); err != nil {
		t.Fatal(err)
	}
}
