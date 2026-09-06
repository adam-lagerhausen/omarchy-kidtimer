package enforcer

import (
	"bytes"
	"testing"
)

func TestWaylandStringRoundTrip(t *testing.T) {
	for _, s := range []string{"", "a", "wl_seat", "ext_idle_notifier_v1"} {
		buf := putString(nil, s)
		got, off, ok := getString(buf, 0)
		if !ok || got != s || off != len(buf) {
			t.Fatalf("%q: got %q off=%d ok=%v len=%d", s, got, off, ok, len(buf))
		}
		if len(buf)%4 != 0 {
			t.Fatalf("pad %q len=%d", s, len(buf))
		}
	}
}

func TestEncodeMsgRoundTrip(t *testing.T) {
	body := putU32(nil, 7)
	raw := encodeMsg(1, wlDisplayGetRegistry, body)
	got, err := readMsg(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if got.id != 1 || got.opcode != wlDisplayGetRegistry {
		t.Fatalf("hdr %+v", got)
	}
	v, _, ok := getU32(got.body, 0)
	if !ok || v != 7 {
		t.Fatalf("body %v", got.body)
	}
}
