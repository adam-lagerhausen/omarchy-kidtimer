package dial

import (
	"os"
	"path/filepath"
	"testing"

	"kidtimer/daemon/internal/reverse"
)

func TestSaveSessionDoesNotFollowPredictableTmp(t *testing.T) {
	dir := t.TempDir()
	path := sessionPath(dir)
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("must survive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path+".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := SaveSession(dir, reverse.SessionFile{Parent: "127.0.0.1:8743", Ticket: "t", ID: "kid-1"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(victim)
	if err != nil || string(got) != "must survive" {
		t.Fatalf("wrote through tmp symlink: %s %v", got, err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}
	s, ok, err := LoadSession(dir)
	if err != nil || !ok || s.Ticket != "t" || s.ID != "kid-1" {
		t.Fatalf("%+v %v %v", s, ok, err)
	}
}
