package advertise

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestParseTXT(t *testing.T) {
	id, name := ParseTXT([]string{"id=abc", "name=sam-box"})
	if id != "abc" || name != "sam-box" {
		t.Fatalf("%s %s", id, name)
	}
}

func TestKidURL(t *testing.T) {
	url := KidURL([]net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("8.8.8.8"), net.ParseIP("192.168.1.20")}, 8742)
	if url != "http://192.168.1.20:8742" {
		t.Fatalf("url %s", url)
	}
	if KidURL([]net.IP{net.ParseIP("8.8.8.8")}, 8742) != "" {
		t.Fatal("public")
	}
	got := KidURL([]net.IP{net.ParseIP("10.0.2.15"), net.ParseIP("100.64.1.2")}, 8742)
	if got != "http://100.64.1.2:8742" {
		t.Fatalf("prefer CGNAT over QEMU NAT: %s", got)
	}
}

func TestURLHousehold(t *testing.T) {
	if !URLHousehold("http://192.168.1.20:8742") || !URLHousehold("http://100.64.1.2:8742") || !URLHousehold("http://127.0.0.1:9") {
		t.Fatal("household")
	}
	if URLHousehold("http://8.8.8.8:8742") || URLHousehold("http://1.1.1.1:8742") {
		t.Fatal("public")
	}
}

func TestAdvertisable(t *testing.T) {
	up := net.Interface{Flags: net.FlagUp | net.FlagMulticast}
	if !Advertisable(up, []net.IP{net.ParseIP("192.168.1.20")}) {
		t.Fatal("lan")
	}
	tun := net.Interface{Flags: net.FlagUp | net.FlagPointToPoint}
	if !Advertisable(tun, []net.IP{net.ParseIP("100.64.1.2")}) {
		t.Fatal("tailscale p-t-p")
	}
	pub := net.Interface{Flags: net.FlagUp | net.FlagMulticast}
	if Advertisable(pub, []net.IP{net.ParseIP("8.8.8.8")}) {
		t.Fatal("public")
	}
	down := net.Interface{Flags: net.FlagMulticast}
	if Advertisable(down, []net.IP{net.ParseIP("192.168.1.20")}) {
		t.Fatal("down")
	}
}

func TestMachineIDStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "machine-id")
	a, err := MachineID(path)
	if err != nil || a == "" {
		t.Fatalf("%s %v", a, err)
	}
	b, err := MachineID(path)
	if err != nil || b != a {
		t.Fatalf("stable %s %s", a, b)
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) == "" {
		t.Fatal("wrote")
	}
}

func TestPortOf(t *testing.T) {
	n, err := PortOf("127.0.0.1:8742")
	if err != nil || n != 8742 {
		t.Fatalf("%d %v", n, err)
	}
}

func TestKidServicesIncludeLegacy(t *testing.T) {
	got := KidServices()
	if len(got) != 2 || got[0] != ServiceType || got[1] != ServiceLegacy {
		t.Fatalf("%v", got)
	}
}

func TestBrowseKidsRequestsBothServices(t *testing.T) {
	seen := map[string]bool{}
	one := func(ctx context.Context, service string, out chan<- Found) error {
		seen[service] = true
		if service == ServiceLegacy {
			select {
			case out <- Found{ID: "kid-1", Name: "testMax", URL: "http://192.168.1.20:8742"}:
			case <-ctx.Done():
			}
		}
		return nil
	}
	ch := make(chan Found, 4)
	if err := browseKids(context.Background(), ch, one); err != nil {
		t.Fatal(err)
	}
	close(ch)
	if !seen[ServiceType] || !seen[ServiceLegacy] {
		t.Fatalf("services %v", seen)
	}
	var got Found
	for f := range ch {
		got = f
	}
	if got.ID != "kid-1" || got.Name != "testMax" {
		t.Fatalf("found %+v", got)
	}
}
