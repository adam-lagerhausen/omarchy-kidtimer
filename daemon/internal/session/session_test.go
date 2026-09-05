package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLoginctlPrefersActive(t *testing.T) {
	raw := "2 1000 sam seat0 tty1 active wayland -\n3 0 gdm seat0 tty7 active wayland -\n"
	uid, err := ParseLoginctl(raw)
	if err != nil || uid != 1000 {
		t.Fatalf("uid=%d err=%v", uid, err)
	}
}

func TestUIDFromHyprRootsSkipsZero(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "0", "hypr"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "1001", "hypr"), 0o755); err != nil {
		t.Fatal(err)
	}
	uid, err := UIDFromHyprRoots(root)
	if err != nil || uid != 1001 {
		t.Fatalf("uid=%d err=%v", uid, err)
	}
}
