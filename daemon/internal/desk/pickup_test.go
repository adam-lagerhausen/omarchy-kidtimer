package desk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"kidtimer/daemon/internal/pin"
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

func TestDeskShareSecondParentGrant(t *testing.T) {
	parentA := t.TempDir()
	parentB := t.TempDir()
	kidHome := t.TempDir()
	b := openPickupBank(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	httpA, sessA := startDesk(t, ctx, parentA)
	httpB, sessB := startDesk(t, ctx, parentB)
	go func() {
		_ = dial.Run(ctx, dial.Config{
			Home:      kidHome,
			Name:      "testMax",
			Bank:      b,
			Endpoints: []reverse.Endpoint{reverse.Endpoint(sessA), reverse.Endpoint(sessB)},
		})
	}()
	var first reverse.Member
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		doc := getHousehold(t, httpA)
		if len(doc.Kids) == 1 && doc.Kids[0].Name == "testMax" && doc.Kids[0].Live {
			first = doc.Kids[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if first.ID == "" || !first.Live {
		t.Fatalf("first desk: %+v", getHousehold(t, httpA))
	}
	var seen reverse.Seen
	for time.Now().Before(deadline) {
		doc := getHousehold(t, httpB)
		if len(doc.Kids) == 0 && len(doc.Seen) == 1 && doc.Seen[0].Claimed && doc.Seen[0].ID == first.ID {
			seen = doc.Seen[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if seen.ID == "" {
		t.Fatalf("second desk claimed: %+v", getHousehold(t, httpB))
	}
	before := groupRemaining(t, first.Status, "fun")
	adopt := postJSONKey(t, "http://"+httpB+"/v1/adopt", map[string]any{"id": string(seen.ID)}, "share-adopt")
	if adopt["kids"] == nil {
		t.Fatalf("adopt: %v", adopt)
	}
	var shared reverse.Member
	for time.Now().Before(deadline) {
		doc := getHousehold(t, httpB)
		if len(doc.Kids) == 1 && doc.Kids[0].ID == first.ID && doc.Kids[0].Live && len(doc.Seen) == 0 {
			shared = doc.Kids[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !shared.Live {
		t.Fatalf("second desk live: %+v", getHousehold(t, httpB))
	}
	grantB := postJSONKey(t, "http://"+httpB+"/v1/kids/"+string(shared.ID)+"/grants", map[string]any{
		"group": "fun", "seconds": 600, "reason": "+10",
	}, "share-b")
	afterB := int(grantB["remaining"].(float64))
	if afterB <= before {
		t.Fatalf("second grant %d -> %d", before, afterB)
	}
	still := getHousehold(t, httpA)
	if len(still.Kids) != 1 || !still.Kids[0].Live || still.Kids[0].ID != first.ID {
		t.Fatalf("first desk lost the box: %+v", still)
	}
	grantA := postJSONKey(t, "http://"+httpA+"/v1/kids/"+string(first.ID)+"/grants", map[string]any{
		"group": "fun", "seconds": 600, "reason": "+10",
	}, "share-a")
	afterA := int(grantA["remaining"].(float64))
	if afterA <= afterB {
		t.Fatalf("first grant %d -> %d", afterB, afterA)
	}
}

func TestDeskShareKeepsFirstOverlayPIN(t *testing.T) {
	parentA := t.TempDir()
	parentB := t.TempDir()
	kidHome := t.TempDir()
	hashA, err := pin.Hash("1111")
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := pin.Hash("2222")
	if err != nil {
		t.Fatal(err)
	}
	if err := household.WritePin(parentA, hashA); err != nil {
		t.Fatal(err)
	}
	if err := household.WritePin(parentB, hashB); err != nil {
		t.Fatal(err)
	}
	b, parentTok, askTok := openPickupBankWithAsk(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	httpA, sessA := startDesk(t, ctx, parentA)
	httpB, sessB := startDesk(t, ctx, parentB)
	go func() {
		_ = dial.Run(ctx, dial.Config{
			Home:      kidHome,
			Name:      "testMax",
			Bank:      b,
			Endpoints: []reverse.Endpoint{reverse.Endpoint(sessA), reverse.Endpoint(sessB)},
		})
	}()
	var first reverse.Member
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		doc := getHousehold(t, httpA)
		if len(doc.Kids) == 1 && doc.Kids[0].Name == "testMax" && doc.Kids[0].Live {
			first = doc.Kids[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if first.ID == "" || !first.Live {
		t.Fatalf("first desk: %+v", getHousehold(t, httpA))
	}
	var pinSet bool
	for time.Now().Before(deadline) {
		st, err := b.Status(parentTok)
		if err == nil && st.ParentPinSet {
			pinSet = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !pinSet {
		t.Fatal("first desk did not push overlay pin")
	}
	var seen reverse.Seen
	for time.Now().Before(deadline) {
		doc := getHousehold(t, httpB)
		if len(doc.Kids) == 0 && len(doc.Seen) == 1 && doc.Seen[0].Claimed && doc.Seen[0].ID == first.ID {
			seen = doc.Seen[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if seen.ID == "" {
		t.Fatalf("second desk claimed: %+v", getHousehold(t, httpB))
	}
	before := groupRemaining(t, first.Status, "fun")
	adopt := postJSONKey(t, "http://"+httpB+"/v1/adopt", map[string]any{"id": string(seen.ID)}, "share-pin-adopt")
	if adopt["kids"] == nil {
		t.Fatalf("adopt: %v", adopt)
	}
	var shared reverse.Member
	for time.Now().Before(deadline) {
		doc := getHousehold(t, httpB)
		if len(doc.Kids) == 1 && doc.Kids[0].ID == first.ID && doc.Kids[0].Live && len(doc.Seen) == 0 {
			shared = doc.Kids[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !shared.Live {
		t.Fatalf("second desk live: %+v", getHousehold(t, httpB))
	}
	grantB := postJSONKey(t, "http://"+httpB+"/v1/kids/"+string(shared.ID)+"/grants", map[string]any{
		"group": "fun", "seconds": 600, "reason": "+10",
	}, "share-pin-b")
	afterB := int(grantB["remaining"].(float64))
	if afterB <= before {
		t.Fatalf("second grant %d -> %d", before, afterB)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := b.PinGrant(askTok, "2222", 60); !errors.Is(err, bank.ErrForbidden) {
		t.Fatalf("share overwrote overlay pin: %v", err)
	}
	if _, err := b.PinGrant(askTok, "1111", 60); err != nil {
		t.Fatalf("first desk overlay pin: %v", err)
	}
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
	return postJSONKey(t, url, body, "test-grant")
}

func postJSONKey(t *testing.T, url string, body map[string]any, key string) map[string]any {
	t.Helper()
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
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
	b, _, _ := openPickupBankWithAsk(t)
	return b
}

func openPickupBankWithAsk(t *testing.T) (*bank.Bank, *bank.Token, *bank.Token) {
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
	_, parent, err := b.SeedParent("test")
	if err != nil {
		t.Fatal(err)
	}
	_, askTok, err := b.Mint(parent, bank.MintSpec{Name: "kid-bar", Kind: bank.KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	return b, parent, askTok
}
