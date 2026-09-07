package desk

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

func TestScanDoesNotPairOverHTTP(t *testing.T) {
	kid := newFakeKid(t, "kid-1", "testMax")
	path := household.Path(t.TempDir())
	if err := household.Save(path, nil); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(path)
	r.replaceSeen([]reverse.Seen{{ID: "kid-1", Name: "testMax", URL: kid.URL, Claimed: true}})
	got, err := household.Load(path)
	if err != nil || len(got) != 0 {
		t.Fatalf("scan must not write kids.json: %+v %v", got, err)
	}
	hh := r.Household(got)
	if len(hh.Seen) != 1 || !hh.Seen[0].Claimed {
		t.Fatalf("seen %+v", hh.Seen)
	}
}

func TestAcceptOfferWritesHousehold(t *testing.T) {
	path := household.Path(t.TempDir())
	if err := household.Save(path, nil); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(path)
	acc, rej := r.acceptOffer(reverse.Offer{ID: "kid-1", Name: "testMax"})
	if rej != nil || acc == nil || acc.Ticket == "" {
		t.Fatalf("accept %+v %+v", acc, rej)
	}
	got, err := household.Load(path)
	if err != nil || len(got) != 1 || got[0].ID != "kid-1" || got[0].Name != "testMax" || got[0].Token != "" {
		t.Fatalf("household %+v %v", got, err)
	}
}

func TestAdoptTakeover(t *testing.T) {
	kid := newFakeKid(t, "kid-1", "testMax")
	resp, err := http.Post(kid.URL+"/v1/pair", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("pair %d", resp.StatusCode)
	}
	secondPath := household.Path(t.TempDir())
	if err := household.Save(secondPath, nil); err != nil {
		t.Fatal(err)
	}
	second := NewRegistry(secondPath)
	second.replaceSeen([]reverse.Seen{{ID: "kid-1", Name: "testMax", URL: kid.URL, Claimed: true}})
	if err := second.Adopt(context.Background(), "kid-1", kid.URL); err != nil {
		t.Fatal(err)
	}
	got, err := household.Load(secondPath)
	if err != nil || len(got) != 1 || got[0].Token != "taken" {
		t.Fatalf("adopt %+v %v", got, err)
	}
	if statusOK(context.Background(), kid.URL, "secret") {
		t.Fatal("old parent-pair")
	}
	if !statusOK(context.Background(), kid.URL, "taken") {
		t.Fatal("new parent-pair")
	}
}

func TestHouseholdUsesRecordNameWhenOffline(t *testing.T) {
	path := household.Path(t.TempDir())
	rec := reverse.Record{
		ID: "kid-1", Name: "testMax", PairedAt: time.Now().UTC(),
	}
	if err := household.Save(path, nil); err != nil {
		t.Fatal(err)
	}
	if err := household.Save(path, []reverse.Record{rec}); err != nil {
		t.Fatal(err)
	}
	hh := NewRegistry(path).Household([]reverse.Record{rec})
	if len(hh.Kids) != 1 || hh.Kids[0].Name != "testMax" || hh.Kids[0].Live {
		t.Fatalf("household %+v", hh.Kids)
	}
}

func TestAdoptPrefersKidName(t *testing.T) {
	kid := newFakeKid(t, "kid-1", "testMax")
	resp, err := http.Post(kid.URL+"/v1/pair", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("pair %d", resp.StatusCode)
	}
	path := household.Path(t.TempDir())
	if err := household.Save(path, nil); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(path)
	r.replaceSeen([]reverse.Seen{{Name: "100.82.187.5", URL: kid.URL, Claimed: true}})
	if err := r.Adopt(context.Background(), "", kid.URL); err != nil {
		t.Fatal(err)
	}
	got, err := household.Load(path)
	if err != nil || len(got) != 1 || got[0].Name != "testMax" {
		t.Fatalf("adopt name %+v %v", got, err)
	}
}

func newFakeKid(t *testing.T, id, name string) *httptest.Server {
	t.Helper()
	var paired atomic.Bool
	var token atomic.Value
	token.Store("secret")
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/status" && r.Method == http.MethodGet:
			want := "Bearer " + token.Load().(string)
			if r.Header.Get("Authorization") == "" || r.Header.Get("Authorization") != want {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"kid_name": name, "groups": map[string]int{"fun": 60}})
		case r.URL.Path == "/v1/asks":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"asks":[]}`))
		case r.URL.Path == "/v1/look":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		case r.URL.Path == "/v1/pair" && r.Method == http.MethodPost:
			if paired.Load() {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"error":"conflict"}`))
				return
			}
			paired.Store(true)
			token.Store("secret")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "name": name, "token": "secret"})
		case r.URL.Path == "/v1/reclaim" && r.Method == http.MethodPost:
			if !paired.Load() {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"error":"conflict"}`))
				return
			}
			token.Store("taken")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "name": name, "token": "taken"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func statusOK(ctx context.Context, rawURL, token string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(rawURL, "/")+"/v1/status", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK
}
