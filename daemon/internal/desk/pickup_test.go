package desk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"kidtimer/daemon/internal/bank"
	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/dial"
	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/httpapi"
	"kidtimer/daemon/internal/reverse"
)

func TestEmptyHousehold(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	httpAddr, _ := startDesk(t, ctx, t.TempDir())
	doc := getHousehold(t, httpAddr)
	if doc.Kids == nil || len(doc.Kids) != 0 {
		t.Fatalf("empty: %+v", doc.Kids)
	}
}

func TestDeskDialGrant(t *testing.T) {
	parentHome := t.TempDir()
	kidHome := t.TempDir()
	b := openPickupBank(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	httpAddr, sessAddr := startDesk(t, ctx, parentHome)
	doc := getHousehold(t, httpAddr)
	if len(doc.Kids) != 0 {
		t.Fatalf("pre: %+v", doc.Kids)
	}
	go func() {
		_ = dial.Run(ctx, dial.Config{
			Home:      kidHome,
			Name:      "testMax",
			Bank:      b,
			Endpoints: []reverse.Endpoint{reverse.Endpoint(sessAddr)},
		})
	}()
	var kid reverse.Member
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		doc = getHousehold(t, httpAddr)
		if len(doc.Kids) == 1 && doc.Kids[0].Name == "testMax" && doc.Kids[0].Live {
			kid = doc.Kids[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if kid.ID == "" || !kid.Live || kid.Name != "testMax" {
		t.Fatalf("live: %+v", doc)
	}
	before := groupRemaining(t, kid.Status, "fun")
	grant := postJSON(t, "http://"+httpAddr+"/v1/kids/"+string(kid.ID)+"/grants", map[string]any{
		"group": "fun", "seconds": 600, "reason": "+10",
	})
	if grant["remaining"] == nil {
		t.Fatalf("grant: %v", grant)
	}
	after := int(grant["remaining"].(float64))
	if after <= before {
		t.Fatalf("remaining %d -> %d", before, after)
	}
}

func TestDeskDialAttachGrant(t *testing.T) {
	parentHome := t.TempDir()
	kidHome := t.TempDir()
	b := openPickupBank(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: httpapi.New(b).Handler()}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	httpAddr, sessAddr := startDesk(t, ctx, parentHome)
	go func() {
		_ = dial.Run(ctx, dial.Config{
			Home:      kidHome,
			Name:      "testMax",
			BankHTTP:  ln.Addr().String(),
			Endpoints: []reverse.Endpoint{reverse.Endpoint(sessAddr)},
		})
	}()
	var kid reverse.Member
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		doc := getHousehold(t, httpAddr)
		if len(doc.Kids) == 1 && doc.Kids[0].Name == "testMax" && doc.Kids[0].Live {
			kid = doc.Kids[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if kid.ID == "" || !kid.Live {
		t.Fatalf("live: %+v", kid)
	}
	grant := postJSON(t, "http://"+httpAddr+"/v1/kids/"+string(kid.ID)+"/grants", map[string]any{
		"group": "fun", "seconds": 600, "reason": "+10",
	})
	if grant["remaining"] == nil {
		t.Fatalf("grant: %v", grant)
	}
}

func TestDeskDialAskShowsOnHousehold(t *testing.T) {
	parentHome := t.TempDir()
	kidHome := t.TempDir()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	cfg, err := config.ParseFile(filepath.Join(root, "packaging", "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := bank.Open(filepath.Join(t.TempDir(), "ledger.sqlite"), cfg, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	_, parent, err := b.SeedParent("test")
	if err != nil {
		t.Fatal(err)
	}
	_, askTok, err := b.Mint(parent, bank.MintSpec{Name: "kid-bar", Kind: bank.KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := b.CreateAsk(askTok, "fun", 1800, "more time")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	httpAddr, sessAddr := startDesk(t, ctx, parentHome)
	go func() {
		_ = dial.Run(ctx, dial.Config{
			Home:      kidHome,
			Name:      "testMax",
			Bank:      b,
			Endpoints: []reverse.Endpoint{reverse.Endpoint(sessAddr)},
		})
	}()
	deadline := time.Now().Add(3 * time.Second)
	var last reverse.Household
	for time.Now().Before(deadline) {
		last = getHousehold(t, httpAddr)
		if len(last.Kids) != 1 || last.Kids[0].Name != "testMax" || !last.Kids[0].Live {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		var asks []bank.Ask
		if err := json.Unmarshal(last.Kids[0].Asks, &asks); err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if len(asks) == 1 && asks[0].ID == ask.ID && asks[0].Seconds == 1800 && asks[0].Status == bank.AskPending {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("household missing pending ask from testMax: %+v", last)
}

func TestDialParentRoleDoesNotOffer(t *testing.T) {
	home := t.TempDir()
	if err := household.WriteRole(home, reverse.RoleParent); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	bankAddr := ln.Addr().String()
	_ = ln.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := dial.Run(ctx, dial.Config{
		Home:      home,
		BankHTTP:  bankAddr,
		Endpoints: []reverse.Endpoint{"127.0.0.1:1"},
	}); err != nil {
		t.Fatal(err)
	}
	ln, err = net.Listen("tcp", bankAddr)
	if err != nil {
		t.Fatalf("bank bound: %v", err)
	}
	_ = ln.Close()
}

func startDesk(t *testing.T, ctx context.Context, home string) (httpAddr, sessAddr string) {
	t.Helper()
	ready := make(chan [2]string, 1)
	errCh := make(chan error, 1)
	go func() {
		errCh <- Run(ctx, Config{
			Home:        home,
			HTTPAddr:    "127.0.0.1:0",
			SessionAddr: "127.0.0.1:0",
			SkipScan:    true,
			OnListen: func(h, s string) {
				ready <- [2]string{h, s}
			},
		})
	}()
	select {
	case addrs := <-ready:
		return addrs[0], addrs[1]
	case err := <-errCh:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		t.Fatal("desk listen")
	}
	return "", ""
}

func getHousehold(t *testing.T, httpAddr string) reverse.Household {
	t.Helper()
	resp, err := http.Get("http://" + httpAddr + "/v1/household")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("household %d %s", resp.StatusCode, body)
	}
	var doc reverse.Household
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Kids == nil {
		t.Fatal("kids null")
	}
	return doc
}

func postJSON(t *testing.T, url string, body map[string]any) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "test-grant")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("post %d %s", resp.StatusCode, out)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func groupRemaining(t *testing.T, raw json.RawMessage, id string) int {
	t.Helper()
	var st struct {
		Groups map[string]int `json:"groups"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	return st.Groups[id]
}

func openPickupBank(t *testing.T) *bank.Bank {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	cfg, err := config.ParseFile(filepath.Join(root, "packaging", "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := bank.Open(filepath.Join(t.TempDir(), "ledger.sqlite"), cfg, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, _, err := b.SeedParent("test"); err != nil {
		t.Fatal(err)
	}
	return b
}
