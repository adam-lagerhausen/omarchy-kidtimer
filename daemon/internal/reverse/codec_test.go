package reverse

import (
	"testing"
)

func TestParseFrameRejectsTwoPayloads(t *testing.T) {
	_, err := ParseFrame([]byte(`{"kind":"offer","offer":{"id":"a","name":"n"},"resume":{"id":"a","ticket":"t"}}`))
	if err != ErrFramePayload {
		t.Fatalf("got %v", err)
	}
	_, err = ParseFrame([]byte(`{"offer":{"id":"a","name":"n"},"op":{"corr":1,"method":"GET","path":"/v1/status"}}`))
	if err != ErrFramePayload {
		t.Fatalf("untagged two: %v", err)
	}
}

func TestParseEncodeOffer(t *testing.T) {
	raw, err := EncodeFrame(Frame{Offer: &Offer{ID: "a", Name: "testMax"}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := ParseFrame(raw)
	if err != nil || f.Kind != KindOffer || f.Offer == nil || f.Offer.Name != "testMax" {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestHashTicketStable(t *testing.T) {
	if HashTicket("abc") != HashTicket("abc") {
		t.Fatal("stable")
	}
	if HashTicket("abc") == HashTicket("abd") {
		t.Fatal("distinct")
	}
}
