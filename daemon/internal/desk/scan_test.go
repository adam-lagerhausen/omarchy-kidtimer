package desk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kidtimer/daemon/internal/advertise"
	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

func TestScanPairsFakeKid(t *testing.T) {
	kid := newFakeKid(t, "kid-1", "testMax")
	home := t.TempDir()
	path := household.Path(home)
	if err := household.Save(path, nil); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(path)
	r.browse = emitFound(advertise.Found{ID: "kid-1", Name: "testMax", URL: kid.URL})
	r.probe = func(context.Context) []string { return nil }
	r.scanTick(context.Background())
	got, err := household.Load(path)
	if err != nil || len(got) != 1 || got[0].ID != "kid-1" || got[0].Name != "testMax" || got[0].URL != kid.URL || got[0].Token != "secret" {
		t.Fatalf("kids.json: %+v %v", got, err)
	}
	doc := r.Household(got)
	if len(doc.Kids) != 1 || !doc.Kids[0].Live || doc.Kids[0].Status == nil {
		t.Fatalf("live path: %+v", doc)
	}
}

func TestScanSecondParentConflict(t *testing.T) {
	kid := newFakeKid(t, "kid-1", "testMax")
	first := NewRegistry(household.Path(t.TempDir()))
	if err := household.Save(first.path, nil); err != nil {
		t.Fatal(err)
	}
	first.browse = emitFound(advertise.Found{ID: "kid-1", Name: "testMax", URL: kid.URL})
	first.probe = func(context.Context) []string { return nil }
	first.scanTick(context.Background())
	secondHome := t.TempDir()
	secondPath := household.Path(secondHome)
	if err := household.Save(secondPath, nil); err != nil {
		t.Fatal(err)
	}
	second := NewRegistry(secondPath)
	second.browse = emitFound(advertise.Found{ID: "kid-1", Name: "testMax", URL: kid.URL})
	second.probe = func(context.Context) []string { return nil }
	second.scanTick(context.Background())
	got, err := household.Load(secondPath)
	if err != nil || len(got) != 0 {
		t.Fatalf("second parent must not pair: %+v %v", got, err)
	}
	hh := second.Household(got)
	if len(hh.Seen) != 1 || !hh.Seen[0].Claimed || hh.Seen[0].URL != kid.URL {
		t.Fatalf("seen %+v", hh.Seen)
	}
}

func TestScanDuplicateIDUpdatesURL(t *testing.T) {
	path := household.Path(t.TempDir())
	old := reverse.Record{ID: "kid-1", Name: "testMax", URL: "http://10.0.2.15:8742", Token: "secret", PairedAt: time.Now().UTC()}
	if err := household.Save(path, []reverse.Record{old}); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(path)
	var paired atomic.Int32
	r.pair = func(ctx context.Context, url string) (pairResult, error) {
		paired.Add(1)
		return pairResult{}, nil
	}
	r.browse = emitFound(advertise.Found{ID: "kid-1", Name: "testMax", URL: "http://100.64.1.2:8742"})
	r.probe = func(context.Context) []string { return nil }
	r.scanTick(context.Background())
	got, err := household.Load(path)
	if err != nil || len(got) != 1 || got[0].URL != "http://100.64.1.2:8742" || got[0].Token != "secret" {
		t.Fatalf("update url: %+v %v", got, err)
	}
	if paired.Load() != 0 {
		t.Fatal("must not re-pair known id")
	}
}

func TestScanIgnoresPublicIP(t *testing.T) {
	path := household.Path(t.TempDir())
	if err := household.Save(path, nil); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(path)
	var paired atomic.Int32
	r.pair = func(ctx context.Context, url string) (pairResult, error) {
		paired.Add(1)
		t.Fatalf("paired public %s", url)
		return pairResult{}, nil
	}
	r.browse = emitFound(advertise.Found{ID: "kid-1", Name: "testMax", URL: "http://8.8.8.8:8742"})
	r.probe = func(context.Context) []string { return nil }
	r.scanTick(context.Background())
	got, err := household.Load(path)
	if err != nil || len(got) != 0 {
		t.Fatalf("public: %+v %v", got, err)
	}
	if paired.Load() != 0 {
		t.Fatal("pair")
	}
}

func TestScanConflictUpdatesURLWithToken(t *testing.T) {
	kid := newFakeKid(t, "kid-1", "testMax")
	resp, err := http.Post(kid.URL+"/v1/pair", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("pre-pair %d", resp.StatusCode)
	}
	path := household.Path(t.TempDir())
	if err := household.Save(path, []reverse.Record{{
		ID: "kid-1", Name: "testMax", URL: "http://10.0.2.15:8742", Token: "secret",
	}}); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(path)
	r.browse = func(ctx context.Context, out chan<- advertise.Found) error { return nil }
	r.probe = func(context.Context) []string { return []string{kid.URL} }
	r.scanTick(context.Background())
	got, err := household.Load(path)
	if err != nil || len(got) != 1 || got[0].URL != kid.URL || got[0].Token != "secret" {
		t.Fatalf("409 token match: %+v %v", got, err)
	}
}

func TestScanConflictRecoversLegacyToken(t *testing.T) {
	kid := newFakeKid(t, "kid-1", "testMax")
	resp, err := http.Post(kid.URL+"/v1/pair", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("pre-pair %d", resp.StatusCode)
	}
	share := t.TempDir()
	dir := filepath.Join(share, "kidtimer")
	legacy := filepath.Join(share, "allowance")
	if err := household.Save(household.Path(dir), nil); err != nil {
		t.Fatal(err)
	}
	if err := household.Save(household.Path(legacy), []reverse.Record{{
		ID: "kid-1", Name: "testMax", URL: "http://10.0.2.15:8742", Token: "secret",
	}}); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(household.Path(dir))
	r.browse = func(ctx context.Context, out chan<- advertise.Found) error { return nil }
	r.probe = func(context.Context) []string { return []string{kid.URL} }
	r.scanTick(context.Background())
	got, err := household.Load(household.Path(dir))
	if err != nil || len(got) != 1 || got[0].URL != kid.URL || got[0].Token != "secret" || got[0].ID != "kid-1" {
		t.Fatalf("legacy 409: %+v %v", got, err)
	}
}

func TestScanAdoptTakeover(t *testing.T) {
	kid := newFakeKid(t, "kid-1", "testMax")
	first := NewRegistry(household.Path(t.TempDir()))
	if err := household.Save(first.path, nil); err != nil {
		t.Fatal(err)
	}
	first.browse = emitFound(advertise.Found{ID: "kid-1", Name: "testMax", URL: kid.URL})
	first.probe = func(context.Context) []string { return nil }
	first.scanTick(context.Background())
	secondHome := t.TempDir()
	secondPath := household.Path(secondHome)
	if err := household.Save(secondPath, nil); err != nil {
		t.Fatal(err)
	}
	second := NewRegistry(secondPath)
	second.browse = emitFound(advertise.Found{ID: "kid-1", Name: "testMax", URL: kid.URL})
	second.probe = func(context.Context) []string { return nil }
	second.scanTick(context.Background())
	got, err := household.Load(secondPath)
	if err != nil || len(got) != 0 {
		t.Fatalf("scan must not takeover: %+v %v", got, err)
	}
	hh := second.Household(got)
	if len(hh.Seen) != 1 {
		t.Fatalf("seen %+v", hh.Seen)
	}
	if err := second.Adopt(context.Background(), "kid-1", kid.URL); err != nil {
		t.Fatal(err)
	}
	got, err = household.Load(secondPath)
	if err != nil || len(got) != 1 || got[0].Token != "taken" {
		t.Fatalf("adopt %+v %v", got, err)
	}
	if statusOK(context.Background(), kid.URL, "secret") {
		t.Fatal("old parent-pair")
	}
	if !statusOK(context.Background(), kid.URL, "taken") {
		t.Fatal("new parent-pair")
	}
	resp, err := http.Post(kid.URL+"/v1/pair", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatalf("pair %d", resp.StatusCode)
	}
	hh = second.Household(got)
	if len(hh.Seen) != 0 {
		t.Fatalf("seen after adopt %+v", hh.Seen)
	}
	second.replaceSeen([]reverse.Seen{{ID: "kid-1", Name: "testMax", URL: kid.URL, Claimed: true}})
	hh = second.Household(nil)
	if len(hh.Seen) != 1 {
		t.Fatal("no keeps them listed")
	}
	if err := second.Adopt(context.Background(), "kid-1", kid.URL); err != nil {
		t.Fatal(err)
	}
}

func TestHouseholdUsesStatusKidName(t *testing.T) {
	kid := newFakeKid(t, "kid-1", "testMax")
	resp, err := http.Post(kid.URL+"/v1/pair", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	path := household.Path(t.TempDir())
	rec := reverse.Record{
		ID: "kid-1", Name: "100.82.187.5", URL: kid.URL, Token: "secret", PairedAt: time.Now().UTC(),
	}
	if err := household.Save(path, []reverse.Record{rec}); err != nil {
		t.Fatal(err)
	}
	hh := NewRegistry(path).Household([]reverse.Record{rec})
	if len(hh.Kids) != 1 || hh.Kids[0].Name != "testMax" {
		t.Fatalf("household name %+v", hh.Kids)
	}
}

func TestAdoptPrefersKidNameOverProbeIP(t *testing.T) {
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

func TestScanProbePairsWithoutBrowse(t *testing.T) {
	kid := newFakeKid(t, "kid-1", "testMax")
	path := household.Path(t.TempDir())
	if err := household.Save(path, nil); err != nil {
		t.Fatal(err)
	}
	r := NewRegistry(path)
	r.browse = func(ctx context.Context, out chan<- advertise.Found) error { return nil }
	r.probe = func(context.Context) []string { return []string{kid.URL} }
	r.scanTick(context.Background())
	got, err := household.Load(path)
	if err != nil || len(got) != 1 || got[0].Name != "testMax" || got[0].Token != "secret" {
		t.Fatalf("probe pair: %+v %v", got, err)
	}
}

func TestHouseholdKidURLsSkipOfflineAndSelf(t *testing.T) {
	old := tailscaleStatusJSON
	t.Cleanup(func() { tailscaleStatusJSON = old })
	tailscaleStatusJSON = func() ([]byte, error) {
		return []byte(`{
			"Self": {"TailscaleIPs": ["100.64.0.1"]},
			"Peer": {
				"a": {"Online": true, "TailscaleIPs": ["100.64.1.2", "8.8.8.8"]},
				"b": {"Online": false, "TailscaleIPs": ["100.64.0.3"]}
			}
		}`), nil
	}
	got := householdKidURLs(context.Background())
	if len(got) != 1 || got[0] != "http://100.64.1.2:8742" {
		t.Fatalf("%v", got)
	}
}

func emitFound(f advertise.Found) func(context.Context, chan<- advertise.Found) error {
	return func(ctx context.Context, out chan<- advertise.Found) error {
		select {
		case out <- f:
		case <-ctx.Done():
		}
		return nil
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
			if r.Header.Get("Authorization") == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			if r.Header.Get("Authorization") != want {
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
