package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStayAwakeHoldRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stay-awake")
	s := &stayAwake{path: path}
	s.hold()
	if !s.ours {
		t.Fatal("ours")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	s.release()
	if s.ours {
		t.Fatal("still ours")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("file should be gone")
	}
}

func TestStayAwakeDoesNotStealExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "stay-awake")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &stayAwake{path: path}
	s.hold()
	if s.ours {
		t.Fatal("must not claim an existing file")
	}
	s.release()
	if _, err := os.Stat(path); err != nil {
		t.Fatal("must leave the existing file")
	}
}
