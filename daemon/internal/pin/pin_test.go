package pin

import "testing"

func TestHashRoundTrip(t *testing.T) {
	h, err := Hash("1234")
	if err != nil {
		t.Fatal(err)
	}
	if !ValidEncoded(h) {
		t.Fatalf("encoded: %s", h)
	}
	if !Verify("1234", h) {
		t.Fatal("verify")
	}
	if Verify("0000", h) {
		t.Fatal("wrong pin matched")
	}
}

func TestValidDigits(t *testing.T) {
	if !ValidDigits("0000") || !ValidDigits("9876") {
		t.Fatal("ok pins")
	}
	for _, s := range []string{"", "12", "123", "12345", "12a4", "abcd"} {
		if ValidDigits(s) {
			t.Fatalf("accepted %q", s)
		}
		if _, err := Hash(s); err == nil {
			t.Fatalf("hashed %q", s)
		}
	}
}
