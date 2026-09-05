package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinSetAndStatus(t *testing.T) {
	dir := t.TempDir()
	home := filepath.Join(dir, "share")
	stdin, err := os.CreateTemp(dir, "pin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.WriteString("4242\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	old := os.Stdin
	os.Stdin = stdin
	t.Cleanup(func() { os.Stdin = old })
	if err := runPin([]string{"set", "-home", home, "-desk", ""}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, "parent-pin"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(raw)), "$argon2id$") {
		t.Fatalf("hash: %s", raw)
	}
	if err := runPin([]string{"status", "-home", home}); err != nil {
		t.Fatal(err)
	}
}
