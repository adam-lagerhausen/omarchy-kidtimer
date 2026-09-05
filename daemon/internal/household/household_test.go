package household

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"kidtimer/daemon/internal/reverse"
)

func TestUpsertKeepsID(t *testing.T) {
	kids := Upsert(nil, reverse.Record{ID: "a", Name: "Sam", TicketHash: "h1", URL: "http://10.0.2.15:8742", Token: "t1"})
	kids = Upsert(kids, reverse.Record{ID: "a", Name: "Sam", URL: "http://100.64.1.2:8742"})
	if len(kids) != 1 || kids[0].TicketHash != "h1" || kids[0].Token != "t1" || kids[0].URL != "http://100.64.1.2:8742" {
		t.Fatalf("%+v", kids)
	}
	kids = Upsert(kids, reverse.Record{ID: "b", Name: "Alex", TicketHash: "h2"})
	if len(kids) != 2 {
		t.Fatalf("two: %+v", kids)
	}
}

func TestSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kidtimer", "kids.json")
	want := []reverse.Record{{ID: "a", Name: "Sam", URL: "http://192.168.1.20:8742", Token: "secret", TicketHash: "hash"}}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(raw, []byte("url")) || !bytes.Contains(raw, []byte("token")) {
		t.Fatalf("pair fields: %s %v", raw, err)
	}
	got, err := Load(path)
	if err != nil || len(got) != 1 || got[0].ID != want[0].ID || got[0].TicketHash != want[0].TicketHash || got[0].URL != want[0].URL || got[0].Token != want[0].Token {
		t.Fatalf("%+v %v", got, err)
	}
	missing, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || missing != nil {
		t.Fatalf("missing %v %v", missing, err)
	}
	emptyPath := filepath.Join(t.TempDir(), "empty.json")
	if err := Save(emptyPath, nil); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(emptyPath)
	if err != nil || !bytes.Contains(raw, []byte(`"kids": []`)) {
		t.Fatalf("empty array: %s %v", raw, err)
	}
}

func TestAdoptKeepsURLAndToken(t *testing.T) {
	kids, err := Adopt([]byte(`{"kids":[{"id":"a","name":"Sam","url":"http://10.0.2.15:8742","token":"secret","ticket_hash":"h"}]}`))
	if err != nil || len(kids) != 1 || kids[0].ID != "a" || kids[0].TicketHash != "h" || kids[0].Name != "Sam" || kids[0].URL != "http://10.0.2.15:8742" || kids[0].Token != "secret" {
		t.Fatalf("%+v %v", kids, err)
	}
}

func TestRole(t *testing.T) {
	home := t.TempDir()
	role, err := LoadRole(home)
	if err != nil || role != reverse.RoleNone {
		t.Fatalf("%v %v", role, err)
	}
	if err := WriteRole(home, reverse.RoleParent); err != nil {
		t.Fatal(err)
	}
	role, err = LoadRole(home)
	if err != nil || role != reverse.RoleParent {
		t.Fatalf("%v %v", role, err)
	}
}
