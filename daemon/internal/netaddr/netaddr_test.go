package netaddr

import (
	"net"
	"testing"
)

func TestIsPrivate(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"192.168.1.20", true},
		{"10.0.0.2", true},
		{"172.16.5.1", true},
		{"169.254.1.1", true},
		{"fd12:3456::1", true},
		{"fe80::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"0.0.0.0", false},
		{"::", false},
		{"100.64.1.2", true},
	}
	for _, tc := range cases {
		if got := IsPrivate(net.ParseIP(tc.ip)); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.ip, got, tc.want)
		}
	}
	if IsPrivate(nil) {
		t.Fatal("nil")
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		ip   string
		want Class
	}{
		{"8.8.8.8", ClassPublic},
		{"1.1.1.1", ClassPublic},
		{"127.0.0.1", ClassLoopback},
		{"::1", ClassLoopback},
		{"192.168.1.20", ClassPrivate},
		{"10.0.2.15", ClassPrivate},
		{"172.16.5.1", ClassPrivate},
		{"fd12:3456::1", ClassPrivate},
		{"100.64.0.1", ClassCGNAT},
		{"100.127.255.254", ClassCGNAT},
		{"100.63.255.255", ClassPublic},
		{"100.128.0.1", ClassPublic},
		{"169.254.1.1", ClassLinkLocal},
		{"fe80::1", ClassLinkLocal},
	}
	for _, tc := range cases {
		if got := Classify(net.ParseIP(tc.ip)); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.ip, got, tc.want)
		}
	}
}

func TestPeerIP(t *testing.T) {
	if !PeerIP("192.168.1.9:4321").Equal(net.ParseIP("192.168.1.9")) {
		t.Fatal("hostport")
	}
	if PeerIP("") != nil || PeerIP("nope") != nil {
		t.Fatal("empty")
	}
}

func TestParseListen(t *testing.T) {
	one, err := ParseListen("127.0.0.1:8742")
	if err != nil || len(one) != 1 || one[0] != "127.0.0.1:8742" {
		t.Fatalf("%v %v", one, err)
	}
	explicit, err := ParseListen("0.0.0.0:8742")
	if err != nil || len(explicit) != 1 || explicit[0] != "0.0.0.0:8742" {
		t.Fatalf("explicit 0.0.0.0: %v %v", explicit, err)
	}
	two, err := ParseListen("127.0.0.1:8742,0.0.0.0:8742")
	if err != nil || len(two) != 2 {
		t.Fatalf("two: %v %v", two, err)
	}
	def, err := ParseListen("")
	if err != nil || len(def) != 1 || def[0] != DefaultListen {
		t.Fatalf("default: %v %v", def, err)
	}
}

func TestTryListenLocalhost(t *testing.T) {
	ln, err := TryListen("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_ = ln.Close()
}
