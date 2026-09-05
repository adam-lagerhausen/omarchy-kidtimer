package dial

import (
	"os"
	"path/filepath"
	"testing"

	"kidtimer/daemon/internal/reverse"
)

func TestLoadHintEndpoints(t *testing.T) {
	home := t.TempDir()
	err := os.WriteFile(filepath.Join(home, "parent-endpoints"), []byte(`
192.168.1.55:8743
100.64.1.2:8743
`), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	got := loadHintEndpoints(home)
	if len(got) != 2 || got[0] != reverse.Endpoint("192.168.1.55:8743") || got[1] != reverse.Endpoint("100.64.1.2:8743") {
		t.Fatalf("%v", got)
	}
}
