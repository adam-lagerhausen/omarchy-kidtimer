package desk

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
	"kidtimer/fonts"
)

func TestAskCardTextMatchesParentPanel(t *testing.T) {
	if got := askCardText("Ada", 1800, false); got != "Ada asked for 30 more minutes" {
		t.Fatalf("minutes: %q", got)
	}
	if got := askCardText("Ada", 600, false); got != "Ada asked for 10 more minutes" {
		t.Fatalf("ten: %q", got)
	}
	if got := askCardText("Ada", 1800, true); got != "Ada asked to unlock for 30 more minutes" {
		t.Fatalf("locked: %q", got)
	}
	if got := askCardText("Ada", 0, false); got != "Ada asked for 10 more minutes" {
		t.Fatalf("zero: %q", got)
	}
}

func TestKitchenAsksSkipDecided(t *testing.T) {
	hh := reverse.Household{Kids: []reverse.Member{{
		ID:   "kid-1",
		Name: "Ada",
		Asks: json.RawMessage(`[{"id":"a1","seconds":600,"status":"pending"},{"id":"a2","seconds":600,"status":"denied"}]`),
	}}}
	got := kitchenAsks(hh)
	if len(got) != 1 || got[0].ID != "a1" || got[0].Text != "Ada asked for 10 more minutes" {
		t.Fatalf("pending: %+v", got)
	}
}

func TestKitchenPageIsParentLook(t *testing.T) {
	r := testRegistry(t)
	rec := doReq(t, r.Handler(), "GET", "/", "127.0.0.1:9", nil)
	if rec.Code != 200 {
		t.Fatalf("page %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, need := range []string{
		"JetBrains Mono",
		"DENY",
		"APPROVE",
		"Same card as the panel",
		"house network",
		"border-radius: 0",
		"#000000",
	} {
		if !strings.Contains(body, need) {
			t.Fatalf("page missing %q", need)
		}
	}
	for _, banned := range []string{"Fun", "School", "TODAY", "login", "password", "overflow"} {
		if strings.Contains(body, banned) {
			t.Fatalf("page has %q", banned)
		}
	}
	font := doReq(t, r.Handler(), "GET", "/font.ttf", "192.168.1.20:9", nil)
	if font.Code != 200 {
		t.Fatalf("font %d", font.Code)
	}
	if !bytes.Equal(font.Body.Bytes(), fonts.MonoRegular) {
		t.Fatal("font bytes")
	}
}

func TestKitchenPeerGate(t *testing.T) {
	r := testRegistry(t)
	h := r.Handler()

	page := doReq(t, h, "GET", "/", "192.168.1.20:9", nil)
	if page.Code != 200 {
		t.Fatalf("lan page %d %s", page.Code, page.Body.String())
	}
	asks := doReq(t, h, "GET", "/v1/kitchen/asks", "10.0.0.8:9", nil)
	if asks.Code != 200 {
		t.Fatalf("lan asks %d %s", asks.Code, asks.Body.String())
	}
	var doc kitchenDoc
	if err := json.Unmarshal(asks.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Asks == nil {
		t.Fatal("asks null")
	}

	hh := doReq(t, h, "GET", "/v1/household", "192.168.1.20:9", nil)
	if hh.Code != http.StatusForbidden || !strings.Contains(hh.Body.String(), "loopback only") {
		t.Fatalf("lan household %d %s", hh.Code, hh.Body.String())
	}
	grant := doReq(t, h, "POST", "/v1/kids/kid-1/grants", "192.168.1.20:9", []byte(`{"group":"fun","seconds":600}`))
	if grant.Code != http.StatusForbidden {
		t.Fatalf("lan grant %d %s", grant.Code, grant.Body.String())
	}
	adopt := doReq(t, h, "POST", "/v1/adopt", "192.168.1.20:9", []byte(`{"url":"http://10.0.0.2:8742"}`))
	if adopt.Code != http.StatusForbidden {
		t.Fatalf("lan adopt %d %s", adopt.Code, adopt.Body.String())
	}

	pub := doReq(t, h, "GET", "/", "8.8.8.8:9", nil)
	if pub.Code != http.StatusForbidden || !strings.Contains(pub.Body.String(), "house network only") {
		t.Fatalf("public page %d %s", pub.Code, pub.Body.String())
	}
	pubAsks := doReq(t, h, "GET", "/v1/kitchen/asks", "8.8.8.8:9", nil)
	if pubAsks.Code != http.StatusForbidden || !strings.Contains(pubAsks.Body.String(), "house network only") {
		t.Fatalf("public asks %d %s", pubAsks.Code, pubAsks.Body.String())
	}
	pubDecide := doReq(t, h, "POST", "/v1/kitchen/asks/kid-1/a1/decide", "8.8.8.8:9", []byte(`{"decision":"approve"}`))
	if pubDecide.Code != http.StatusForbidden || !strings.Contains(pubDecide.Body.String(), "house network only") {
		t.Fatalf("public decide %d %s", pubDecide.Code, pubDecide.Body.String())
	}
	cgnat := doReq(t, h, "GET", "/", "100.64.1.2:9", nil)
	if cgnat.Code != http.StatusForbidden || !strings.Contains(cgnat.Body.String(), "house network only") {
		t.Fatalf("cgnat page %d %s", cgnat.Code, cgnat.Body.String())
	}
	loopHH := doReq(t, h, "GET", "/v1/household", "127.0.0.1:9", nil)
	if loopHH.Code != 200 {
		t.Fatalf("loopback household %d %s", loopHH.Code, loopHH.Body.String())
	}
}

func TestDefaultHTTPListensOnLAN(t *testing.T) {
	if DefaultHTTP != "0.0.0.0:8741" {
		t.Fatalf("kitchen phone needs a LAN bind, got %q", DefaultHTTP)
	}
}

func TestKitchenDecideRejectsBadDecision(t *testing.T) {
	r := testRegistry(t)
	rec := doReq(t, r.Handler(), "POST", "/v1/kitchen/asks/kid-1/a1/decide", "192.168.1.20:9", []byte(`{"decision":"maybe"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad decision %d %s", rec.Code, rec.Body.String())
	}
	missing := doReq(t, r.Handler(), "POST", "/v1/kitchen/asks/kid-1/a1/decide", "192.168.1.20:9", []byte(`{"decision":"approve"}`))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing ask %d %s", missing.Code, missing.Body.String())
	}
}

func TestKitchenPageIgnoresStalePollAndInFlightTap(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	script := filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "run-kitchen-page.js")
	cmd := exec.Command(node, script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kitchen page: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("kitchen page: %s", out)
	}
}

func TestKitchenHasAsk(t *testing.T) {
	hh := reverse.Household{Kids: []reverse.Member{{
		ID:     "kid-1",
		Name:   "Ada",
		Status: json.RawMessage(`{"parent_locked":true}`),
		Asks:   json.RawMessage(`[{"id":"a1","seconds":600,"status":"pending"}]`),
	}}}
	got := kitchenAsks(hh)
	if len(got) != 1 || got[0].Text != "Ada asked to unlock for 10 more minutes" {
		t.Fatalf("locked wire: %+v", got)
	}
	if !kitchenHasAsk(hh, "kid-1", "a1") || kitchenHasAsk(hh, "kid-1", "nope") {
		t.Fatal("has ask")
	}
}

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	dir := t.TempDir()
	path := household.Path(dir)
	if err := household.Save(path, nil); err != nil {
		t.Fatal(err)
	}
	return NewRegistry(path)
}

func doReq(t *testing.T, h http.Handler, method, path, remote string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.RemoteAddr = remote
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
