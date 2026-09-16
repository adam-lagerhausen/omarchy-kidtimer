package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"kidtimer/daemon/internal/bank"
	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/look"
)

func TestGrantStatusAsksDecideMint(t *testing.T) {
	h, parent, _ := start(t)

	grant := post(t, h, parent, "/v1/grants", map[string]any{
		"group": "fun", "seconds": 900, "reason": "good afternoon",
	}, "idem-fun")
	if grant.StatusCode != 200 {
		t.Fatalf("grant: %d %s", grant.StatusCode, grant.Body)
	}
	missingKey := post(t, h, parent, "/v1/grants", map[string]any{
		"group": "fun", "seconds": 1, "reason": "no key",
	}, "")
	if missingKey.StatusCode != 400 {
		t.Fatalf("missing idempotency key: %d %s", missingKey.StatusCode, missingKey.Body)
	}
	replay := post(t, h, parent, "/v1/grants", map[string]any{
		"group": "fun", "seconds": 900, "reason": "good afternoon",
	}, "idem-fun")
	if replay.StatusCode != 200 {
		t.Fatalf("replay: %d %s", replay.StatusCode, replay.Body)
	}
	if !asMap(t, replay.Body)["replay"].(bool) {
		t.Fatal("replay flag")
	}
	conflict := post(t, h, parent, "/v1/grants", map[string]any{
		"group": "fun", "seconds": 60, "reason": "other",
	}, "idem-fun")
	if conflict.StatusCode != 409 {
		t.Fatalf("same key different body: %d %s", conflict.StatusCode, conflict.Body)
	}

	st := get(t, h, parent, "/v1/status")
	if st.StatusCode != 200 {
		t.Fatalf("status: %d %s", st.StatusCode, st.Body)
	}
	status := asMap(t, st.Body)
	if status["bedtime_active"].(bool) {
		t.Fatal("afternoon is not bedtime")
	}
	groups := status["groups"].(map[string]any)
	if int(groups["fun"].(float64)) != 4500 {
		t.Fatalf("fun remaining: %v", groups["fun"])
	}
	path := status["path_remaining"].(map[string]any)
	if _, ok := path["school"]; ok {
		t.Fatal("bypass school must not be in path_remaining")
	}
	if _, ok := path["minecraft"]; ok {
		t.Fatal("minecraft is not a clock")
	}
	if int(path["fun"].(float64)) != 4500 {
		t.Fatalf("fun path: %v", path["fun"])
	}
	if _, ok := status["bedtime_in"]; ok {
		t.Fatal("parent-lab bedtime_lock off must omit bedtime_in")
	}
	piles, _ := status["piles"].([]any)
	if len(piles) != 1 || piles[0].(map[string]any)["id"] != "fun" {
		t.Fatalf("seed piles: %v", status["piles"])
	}
	if status["focused_app"] != nil {
		t.Fatalf("focused_app: %v", status["focused_app"])
	}
	if int(status["look_version"].(float64)) < 1 {
		t.Fatalf("look_version: %v", status["look_version"])
	}
	if status["parent_pin_set"] != false {
		t.Fatalf("parent_pin_set: %v", status["parent_pin_set"])
	}
	if status["overlay"] != false {
		t.Fatalf("overlay: %v", status["overlay"])
	}
	today, _ := status["today"].([]any)
	if today == nil {
		t.Fatal("today missing")
	}
	if len(today) != 0 {
		t.Fatalf("fresh today: %v", status["today"])
	}

	mint := post(t, h, parent, "/v1/tokens", map[string]any{
		"name": "kid-bar", "kind": "ask",
	}, "")
	askSecret := asMap(t, mint.Body)["secret"].(string)
	ask := post(t, h, askSecret, "/v1/asks", map[string]any{
		"group": "fun", "seconds": 900, "reason": "one more video",
	}, "")
	if ask.StatusCode != 200 {
		t.Fatalf("ask: %d %s", ask.StatusCode, ask.Body)
	}
	askID := asMap(t, ask.Body)["id"].(string)
	listed := get(t, h, parent, "/v1/asks")
	if listed.StatusCode != 200 {
		t.Fatalf("list asks: %d %s", listed.StatusCode, listed.Body)
	}
	listedAsks := asMap(t, listed.Body)["asks"].([]any)
	if len(listedAsks) != 1 {
		t.Fatalf("pending asks: %s", listed.Body)
	}
	row := listedAsks[0].(map[string]any)
	if row["id"] != askID || row["group"] != "fun" || int(row["seconds"].(float64)) != 900 {
		t.Fatalf("ask wire: %s", listed.Body)
	}
	if _, pascal := row["ID"]; pascal {
		t.Fatalf("PascalCase ask id: %s", listed.Body)
	}
	decide := post(t, h, parent, "/v1/asks/"+askID+"/decide", map[string]any{"decision": "approve"}, "")
	if decide.StatusCode != 200 {
		t.Fatalf("decide: %d %s", decide.StatusCode, decide.Body)
	}
	again := post(t, h, parent, "/v1/asks/"+askID+"/decide", map[string]any{"decision": "approve"}, "")
	if again.StatusCode != 409 {
		t.Fatalf("second decide: %d %s", again.StatusCode, again.Body)
	}
	if !bytes.Contains([]byte(again.Body), []byte("already decided")) {
		t.Fatalf("409 body: %s", again.Body)
	}

	st = get(t, h, parent, "/v1/status")
	groups = asMap(t, st.Body)["groups"].(map[string]any)
	if int(groups["fun"].(float64)) != 5400 {
		t.Fatalf("fun after approve: %v", groups["fun"])
	}
}

func TestPinRoutes(t *testing.T) {
	h, parent, _ := start(t)
	askSecret := asMap(t, post(t, h, parent, "/v1/tokens", map[string]any{"name": "kid-bar", "kind": "ask"}, "").Body)["secret"].(string)
	readSecret := asMap(t, post(t, h, parent, "/v1/tokens", map[string]any{"name": "bar-read", "kind": "read"}, "").Body)["secret"].(string)
	if doJSON(t, h, http.MethodPut, askSecret, "/v1/parent-pin", map[string]any{"pin": "1234"}, "").StatusCode != 403 {
		t.Fatal("ask must not set pin")
	}
	set := doJSON(t, h, http.MethodPut, parent, "/v1/parent-pin", map[string]any{"pin": "1234"}, "")
	if set.StatusCode != 200 {
		t.Fatalf("set pin: %d %s", set.StatusCode, set.Body)
	}
	st := asMap(t, get(t, h, parent, "/v1/status").Body)
	if st["parent_pin_set"] != true {
		t.Fatalf("status pin: %v", st["parent_pin_set"])
	}
	ask := post(t, h, askSecret, "/v1/asks", map[string]any{"group": "fun", "seconds": 600, "reason": "more time"}, "")
	askID := asMap(t, ask.Body)["id"].(string)
	bad := post(t, h, askSecret, "/v1/pin/approve", map[string]any{"pin": "0000", "ask_id": askID}, "")
	if bad.StatusCode != 403 {
		t.Fatalf("wrong pin: %d %s", bad.StatusCode, bad.Body)
	}
	ok := post(t, h, askSecret, "/v1/pin/approve", map[string]any{"pin": "1234", "ask_id": askID}, "")
	if ok.StatusCode != 200 {
		t.Fatalf("approve: %d %s", ok.StatusCode, ok.Body)
	}
	if asMap(t, ok.Body)["status"] != "approved" {
		t.Fatalf("approve body: %s", ok.Body)
	}
	grant := post(t, h, readSecret, "/v1/pin/grant", map[string]any{"pin": "1234", "seconds": 300}, "")
	if grant.StatusCode != 200 {
		t.Fatalf("grant: %d %s", grant.StatusCode, grant.Body)
	}
	if asMap(t, grant.Body)["source"] != "parent-pin" {
		t.Fatalf("grant source: %s", grant.Body)
	}
	parentGrant := post(t, h, parent, "/v1/pin/grant", map[string]any{"pin": "1234", "seconds": 60}, "")
	if parentGrant.StatusCode != 403 {
		t.Fatalf("parent pin grant: %d", parentGrant.StatusCode)
	}
}

func TestBadBearer(t *testing.T) {
	h, parent, _ := start(t)
	res := get(t, h, "nope", "/v1/status")
	if res.StatusCode != 401 {
		t.Fatalf("bad bearer: %d %s", res.StatusCode, res.Body)
	}
	req, _ := http.NewRequest(http.MethodGet, h.URL+"/v1/status", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("missing bearer: %d", resp.StatusCode)
	}
	req, _ = http.NewRequest(http.MethodGet, h.URL+"/v1/status", nil)
	req.Header.Set("Authorization", "bearer "+parent)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("lowercase bearer: %d", resp.StatusCode)
	}
}

func TestPairPrivateFirstWins(t *testing.T) {
	h, parent, _ := start(t)
	if get(t, h, "", "/v1/status").StatusCode != 401 && get(t, h, "nope", "/v1/status").StatusCode != 401 {
		t.Fatal("grant still needs bearer")
	}
	if post(t, h, "", "/v1/grants", map[string]any{"group": "fun", "seconds": 1, "reason": "x"}, "no-auth").StatusCode == 200 {
		t.Fatal("grant without bearer")
	}
	first := post(t, h, "", "/v1/pair", map[string]any{}, "")
	if first.StatusCode != 200 {
		t.Fatalf("pair: %d %s", first.StatusCode, first.Body)
	}
	body := asMap(t, first.Body)
	if body["token"] == "" || body["name"] != "parent-lab" || body["id"] == "" {
		t.Fatalf("pair body: %s", first.Body)
	}
	if get(t, h, body["token"].(string), "/v1/status").StatusCode != 200 {
		t.Fatal("pair token must status")
	}
	if get(t, h, parent, "/v1/status").StatusCode != 200 {
		t.Fatal("bootstrap still works")
	}
	second := post(t, h, "", "/v1/pair", map[string]any{}, "")
	if second.StatusCode != 409 {
		t.Fatalf("second pair: %d %s", second.StatusCode, second.Body)
	}
}

func TestPairPublicForbidden(t *testing.T) {
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := bank.Open(filepath.Join(t.TempDir(), "ledger.sqlite"), cfg, afternoon)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	api := New(b)
	api.MachineID = "machine-1"
	h := api.Handler()
	req := httptest.NewRequest(http.MethodPost, "/v1/pair", strings.NewReader("{}"))
	req.RemoteAddr = "8.8.8.8:9"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("public: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/pair", strings.NewReader("{}"))
	req.RemoteAddr = ""
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("unspecified: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/pair", strings.NewReader("{}"))
	req.RemoteAddr = "192.168.1.9:9"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 404 {
		t.Fatalf("lan: %d %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/pair", strings.NewReader("{}"))
	req.RemoteAddr = "127.0.0.1:9"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("loopback: %d %s", rec.Code, rec.Body.String())
	}
	if asMap(t, rec.Body.String())["id"] != "machine-1" {
		t.Fatalf("id: %s", rec.Body.String())
	}
}

func TestReclaimPrivateRemints(t *testing.T) {
	h, parent, _ := start(t)
	first := post(t, h, "", "/v1/pair", map[string]any{}, "")
	if first.StatusCode != 200 {
		t.Fatalf("pair: %d %s", first.StatusCode, first.Body)
	}
	old := asMap(t, first.Body)["token"].(string)
	again := post(t, h, "", "/v1/reclaim", map[string]any{}, "")
	if again.StatusCode != 200 {
		t.Fatalf("reclaim: %d %s", again.StatusCode, again.Body)
	}
	body := asMap(t, again.Body)
	next := body["token"].(string)
	if next == "" || next == old {
		t.Fatalf("token: %s", again.Body)
	}
	if get(t, h, old, "/v1/status").StatusCode != 401 {
		t.Fatal("old pair")
	}
	if get(t, h, next, "/v1/status").StatusCode != 200 {
		t.Fatal("new pair")
	}
	if get(t, h, parent, "/v1/status").StatusCode != 200 {
		t.Fatal("bootstrap")
	}
	if post(t, h, "", "/v1/pair", map[string]any{}, "").StatusCode != 409 {
		t.Fatal("pair stays 409")
	}
}

func TestGrantAndApproveInvokeResume(t *testing.T) {
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := bank.Open(filepath.Join(t.TempDir(), "ledger.sqlite"), cfg, afternoon)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	secret, _, err := b.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	var resumed []string
	api := New(b)
	api.Resume = func(group string) {
		resumed = append(resumed, group)
	}
	h := httptest.NewServer(api.Handler())
	t.Cleanup(h.Close)

	grant := post(t, h, secret, "/v1/grants", map[string]any{
		"group": "fun", "seconds": 900, "reason": "+15",
	}, "resume-fun")
	if grant.StatusCode != 200 {
		t.Fatalf("grant: %d %s", grant.StatusCode, grant.Body)
	}
	replay := post(t, h, secret, "/v1/grants", map[string]any{
		"group": "fun", "seconds": 900, "reason": "+15",
	}, "resume-fun")
	if replay.StatusCode != 200 || !asMap(t, replay.Body)["replay"].(bool) {
		t.Fatalf("replay: %d %s", replay.StatusCode, replay.Body)
	}
	if len(resumed) != 1 || resumed[0] != "fun" {
		t.Fatalf("grant resume: %v", resumed)
	}

	mint := post(t, h, secret, "/v1/tokens", map[string]any{"name": "kid-bar", "kind": "ask"}, "")
	askSecret := asMap(t, mint.Body)["secret"].(string)
	ask := post(t, h, askSecret, "/v1/asks", map[string]any{
		"group": "fun", "seconds": 900, "reason": "one more video",
	}, "")
	askID := asMap(t, ask.Body)["id"].(string)
	decide := post(t, h, secret, "/v1/asks/"+askID+"/decide", map[string]any{"decision": "approve"}, "")
	if decide.StatusCode != 200 {
		t.Fatalf("decide: %d %s", decide.StatusCode, decide.Body)
	}
	if len(resumed) != 2 || resumed[1] != "fun" {
		t.Fatalf("approve resume: %v", resumed)
	}
}

func TestParentDebitAndAppForbidden(t *testing.T) {
	h, parent, _ := start(t)
	seed := post(t, h, parent, "/v1/grants", map[string]any{
		"group": "fun", "seconds": 300, "reason": "seed",
	}, "seed-300")
	if seed.StatusCode != 200 {
		t.Fatalf("seed: %d %s", seed.StatusCode, seed.Body)
	}
	debit := post(t, h, parent, "/v1/grants", map[string]any{
		"group": "fun", "seconds": -600, "reason": "-10",
	}, "parent-minus10")
	if debit.StatusCode != 200 {
		t.Fatalf("parent debit: %d %s", debit.StatusCode, debit.Body)
	}
	body := asMap(t, debit.Body)
	if int(body["seconds"].(float64)) != -600 || body["reason"] != "-10" || int(body["remaining"].(float64)) != 3300 {
		t.Fatalf("parent debit body: %s", debit.Body)
	}

	appMint := post(t, h, parent, "/v1/tokens", map[string]any{"name": "khan-webhook", "kind": "app"}, "")
	appSecret := asMap(t, appMint.Body)["secret"].(string)
	appDebit := post(t, h, appSecret, "/v1/grants", map[string]any{
		"group": "fun", "seconds": -600, "reason": "-10",
	}, "app-minus10")
	if appDebit.StatusCode != 403 {
		t.Fatalf("app debit: %d %s", appDebit.StatusCode, appDebit.Body)
	}
}

func TestReadAndAppTokenKinds(t *testing.T) {
	h, parent, _ := start(t)
	readMint := post(t, h, parent, "/v1/tokens", map[string]any{"name": "reader", "kind": "read"}, "")
	readSecret := asMap(t, readMint.Body)["secret"].(string)
	appMint := post(t, h, parent, "/v1/tokens", map[string]any{
		"name": "khan-webhook", "kind": "app",
		"max_seconds_per_grant": 600, "max_seconds_per_day": 1800,
	}, "")
	app := asMap(t, appMint.Body)
	if app["groups"].([]any)[0] != "fun" {
		t.Fatalf("app default groups: %v", app["groups"])
	}
	appSecret := app["secret"].(string)

	if get(t, h, readSecret, "/v1/status").StatusCode != 200 {
		t.Fatal("read token should status")
	}
	if post(t, h, readSecret, "/v1/grants", map[string]any{"group": "fun", "seconds": 10, "reason": "x"}, "k").StatusCode != 403 {
		t.Fatal("read token must not grant")
	}
	g := post(t, h, appSecret, "/v1/grants", map[string]any{"group": "fun", "seconds": 10, "reason": "lesson"}, "app-1")
	if g.StatusCode != 200 {
		t.Fatalf("app grant: %d %s", g.StatusCode, g.Body)
	}
	if asMap(t, g.Body)["source"] != "app:khan-webhook" {
		t.Fatalf("source: %s", g.Body)
	}
	if post(t, h, appSecret, "/v1/grants", map[string]any{"group": "missing", "seconds": 10, "reason": "x"}, "app-2").StatusCode != 400 {
		t.Fatal("app must not credit an unknown group")
	}
	if post(t, h, appSecret, "/v1/tokens", map[string]any{"name": "x", "kind": "read"}, "").StatusCode != 403 {
		t.Fatal("app must not mint")
	}
}

func TestParentLock(t *testing.T) {
	h, parent, _ := start(t)
	lock := post(t, h, parent, "/v1/lock", map[string]any{"locked": true}, "")
	if lock.StatusCode != 200 {
		t.Fatalf("lock: %d %s", lock.StatusCode, lock.Body)
	}
	if !asMap(t, lock.Body)["locked"].(bool) {
		t.Fatal("locked body")
	}
	replay := post(t, h, parent, "/v1/lock", map[string]any{"locked": true}, "")
	if replay.StatusCode != 200 {
		t.Fatalf("lock replay: %d %s", replay.StatusCode, replay.Body)
	}
	st := get(t, h, parent, "/v1/status")
	status := asMap(t, st.Body)
	if status["parent_locked"] != true {
		t.Fatalf("parent_locked: %s", st.Body)
	}
	if status["remote_lock"] != false {
		t.Fatalf("remote_lock: %s", st.Body)
	}
	if status["bedtime_start"] != "21:00" || status["bedtime_end"] != "07:00" {
		t.Fatalf("bedtime clocks: %s", st.Body)
	}
	unlock := post(t, h, parent, "/v1/lock", map[string]any{"locked": false}, "")
	if unlock.StatusCode != 200 {
		t.Fatalf("unlock: %d %s", unlock.StatusCode, unlock.Body)
	}
	st = get(t, h, parent, "/v1/status")
	if asMap(t, st.Body)["parent_locked"] != false {
		t.Fatalf("unlocked: %s", st.Body)
	}
}

func TestApproveAskUnlocks(t *testing.T) {
	h, parent, _ := start(t)
	mint := post(t, h, parent, "/v1/tokens", map[string]any{
		"name": "kid-bar", "kind": "ask",
	}, "")
	askSecret := asMap(t, mint.Body)["secret"].(string)
	if post(t, h, parent, "/v1/lock", map[string]any{"locked": true}, "").StatusCode != 200 {
		t.Fatal("lock")
	}
	ask := post(t, h, askSecret, "/v1/asks", map[string]any{
		"group": "fun", "seconds": 1800, "reason": "more time",
	}, "")
	if ask.StatusCode != 200 {
		t.Fatalf("ask: %d %s", ask.StatusCode, ask.Body)
	}
	askID := asMap(t, ask.Body)["id"].(string)
	decide := post(t, h, parent, "/v1/asks/"+askID+"/decide", map[string]any{"decision": "approve"}, "")
	if decide.StatusCode != 200 {
		t.Fatalf("decide: %d %s", decide.StatusCode, decide.Body)
	}
	st := get(t, h, parent, "/v1/status")
	status := asMap(t, st.Body)
	if status["parent_locked"] != false {
		t.Fatalf("approve must unlock: %s", st.Body)
	}
}

func TestLeftoverModeDoesNotChangeRemaining(t *testing.T) {
	h, parent, _ := start(t)
	seed := int(asMap(t, get(t, h, parent, "/v1/status").Body)["groups"].(map[string]any)["fun"].(float64))
	mode := post(t, h, parent, "/v1/mode", map[string]any{"id": "morning"}, "")
	if mode.StatusCode != 200 {
		t.Fatalf("leftover mode: %d %s", mode.StatusCode, mode.Body)
	}
	st := get(t, h, parent, "/v1/status")
	if int(asMap(t, st.Body)["groups"].(map[string]any)["fun"].(float64)) != seed {
		t.Fatalf("mode must not change remaining: %s", st.Body)
	}
	grant := post(t, h, parent, "/v1/grants", map[string]any{
		"group": "fun", "seconds": 60, "reason": "keep",
	}, "keep-fun")
	if grant.StatusCode != 200 {
		t.Fatalf("grant: %d %s", grant.StatusCode, grant.Body)
	}
	again := post(t, h, parent, "/v1/mode", map[string]any{"id": "morning"}, "")
	if again.StatusCode != 200 {
		t.Fatalf("mode again: %d %s", again.StatusCode, again.Body)
	}
	st = get(t, h, parent, "/v1/status")
	if int(asMap(t, st.Body)["groups"].(map[string]any)["fun"].(float64)) != seed+60 {
		t.Fatalf("mode re-tap must not refill: %s", st.Body)
	}
}

func TestPolicyDoesNotRefill(t *testing.T) {
	h, parent, _ := start(t)
	grant := post(t, h, parent, "/v1/grants", map[string]any{
		"group": "fun", "seconds": 90, "reason": "keep",
	}, "keep-policy")
	if grant.StatusCode != 200 {
		t.Fatalf("grant: %d %s", grant.StatusCode, grant.Body)
	}
	before := int(asMap(t, grant.Body)["remaining"].(float64))
	patched := patch(t, h, parent, "/v1/policy", map[string]any{"bedtime_start": "20:00"})
	if patched.StatusCode != 200 {
		t.Fatalf("policy: %d %s", patched.StatusCode, patched.Body)
	}
	pol := asMap(t, patched.Body)
	if pol["bedtime_start"] != "20:00" || pol["bedtime_end"] != "07:00" {
		t.Fatalf("policy bedtime: %s", patched.Body)
	}
	if int(pol["groups"].(map[string]any)["fun"].(float64)) != before {
		t.Fatalf("policy must not refill: %s", patched.Body)
	}
}

func TestPolicyHour12(t *testing.T) {
	h, parent, _ := start(t)
	st := get(t, h, parent, "/v1/status")
	if asMap(t, st.Body)["hour12"] != true {
		t.Fatalf("default hour12: %s", st.Body)
	}
	patched := patch(t, h, parent, "/v1/policy", map[string]any{"hour12": false})
	if patched.StatusCode != 200 {
		t.Fatalf("policy hour12: %d %s", patched.StatusCode, patched.Body)
	}
	if asMap(t, patched.Body)["hour12"] != false {
		t.Fatalf("hour12 off: %s", patched.Body)
	}
	keep := patch(t, h, parent, "/v1/policy", map[string]any{"bedtime_start": "20:00"})
	if keep.StatusCode != 200 {
		t.Fatalf("omit hour12: %d %s", keep.StatusCode, keep.Body)
	}
	got := asMap(t, keep.Body)
	if got["hour12"] != false {
		t.Fatalf("omit keeps hour12: %s", keep.Body)
	}
	if got["bedtime_start"] != "20:00" {
		t.Fatalf("bed still patches: %s", keep.Body)
	}
}

func TestGrantIgnoresClientSource(t *testing.T) {
	h, parent, _ := start(t)
	raw := []byte(`{"group":"fun","seconds":60,"reason":"+10","source":"app:spoof"}`)
	req, err := http.NewRequest(http.MethodPost, h.URL+"/v1/grants", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+parent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "spoof-source")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("grant: %d %s", resp.StatusCode, b)
	}
	if asMap(t, string(b))["source"] != "parent" {
		t.Fatalf("source must come from the token: %s", b)
	}
}

func TestRouteAuthz(t *testing.T) {
	h, parent, _ := start(t)
	askSecret := asMap(t, post(t, h, parent, "/v1/tokens", map[string]any{"name": "kid-bar", "kind": "ask"}, "").Body)["secret"].(string)
	readSecret := asMap(t, post(t, h, parent, "/v1/tokens", map[string]any{"name": "reader", "kind": "read"}, "").Body)["secret"].(string)
	appSecret := asMap(t, post(t, h, parent, "/v1/tokens", map[string]any{"name": "khan-webhook", "kind": "app"}, "").Body)["secret"].(string)
	pending := post(t, h, askSecret, "/v1/asks", map[string]any{"group": "fun", "seconds": 60, "reason": "more"}, "")
	if pending.StatusCode != 200 {
		t.Fatalf("setup ask: %d %s", pending.StatusCode, pending.Body)
	}
	askID := asMap(t, pending.Body)["id"].(string)
	lookDoc := asMap(t, get(t, h, parent, "/v1/look").Body)

	cases := []struct {
		name   string
		method string
		path   string
		body   map[string]any
		idem   string
		want   map[string]int
	}{
		{"grants", http.MethodPost, "/v1/grants", map[string]any{"group": "fun", "seconds": 10, "reason": "x"}, "authz-credit", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 200}},
		{"grants debit", http.MethodPost, "/v1/grants", map[string]any{"group": "fun", "seconds": -600, "reason": "-10"}, "authz-debit", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 403}},
		{"status", http.MethodGet, "/v1/status", nil, "", map[string]int{"parent": 200, "ask": 403, "read": 200, "app": 403}},
		{"create ask", http.MethodPost, "/v1/asks", map[string]any{"group": "fun", "seconds": 60, "reason": "more"}, "", map[string]int{"parent": 403, "ask": 200, "read": 403, "app": 403}},
		{"list asks", http.MethodGet, "/v1/asks", nil, "", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 403}},
		{"decide", http.MethodPost, "/v1/asks/" + askID + "/decide", map[string]any{"decision": "deny"}, "", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 403}},
		{"tokens", http.MethodPost, "/v1/tokens", map[string]any{"name": "extra-read", "kind": "read"}, "", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 403}},
		{"lock", http.MethodPost, "/v1/lock", map[string]any{"locked": false}, "", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 403}},
		{"parent pin", http.MethodPut, "/v1/parent-pin", map[string]any{"pin": "1234"}, "", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 403}},
		{"policy", http.MethodPatch, "/v1/policy", map[string]any{"bedtime_start": "21:00"}, "", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 403}},
		{"get look", http.MethodGet, "/v1/look", nil, "", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 403}},
		{"put look", http.MethodPut, "/v1/look", lookDoc, "", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 403}},
		{"search", http.MethodGet, "/v1/search?q=x&list=fun", nil, "", map[string]int{"parent": 200, "ask": 403, "read": 403, "app": 403}},
	}
	tokens := map[string]string{"parent": parent, "ask": askSecret, "read": readSecret, "app": appSecret}
	for _, tc := range cases {
		for _, kind := range []string{"ask", "read", "app", "parent"} {
			res := doJSON(t, h, tc.method, tokens[kind], tc.path, tc.body, tc.idem+"-"+kind)
			if res.StatusCode != tc.want[kind] {
				t.Fatalf("%s %s: got %d want %d %s", tc.name, kind, res.StatusCode, tc.want[kind], res.Body)
			}
		}
		if doJSON(t, h, tc.method, "nope", tc.path, tc.body, tc.idem+"-bad").StatusCode != 401 {
			t.Fatalf("%s bad bearer", tc.name)
		}
		req, err := http.NewRequest(tc.method, h.URL+tc.path, nil)
		if err != nil {
			t.Fatal(err)
		}
		if tc.body != nil {
			raw, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatal(err)
			}
			req, err = http.NewRequest(tc.method, h.URL+tc.path, bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("%s missing bearer: %d", tc.name, resp.StatusCode)
		}
	}
}

func TestLookGetPut(t *testing.T) {
	h, parent, b := start(t)
	got := get(t, h, parent, "/v1/look")
	if got.StatusCode != 200 {
		t.Fatalf("get look: %d %s", got.StatusCode, got.Body)
	}
	doc := asMap(t, got.Body)
	if int(doc["version"].(float64)) < 1 {
		t.Fatalf("version: %s", got.Body)
	}
	piles := doc["piles"].([]any)
	if len(piles) != 1 || piles[0].(map[string]any)["id"] != "fun" {
		t.Fatalf("seed piles: %s", got.Body)
	}
	if _, ok := doc["catalog"]; ok {
		t.Fatal("catalog dump")
	}
	if _, ok := doc["matchers"]; ok {
		t.Fatal("matchers on parent wire")
	}
	if doc["fun_hours"].(map[string]any)["sat"].(float64) != 7200 {
		t.Fatalf("sat hours: %s", got.Body)
	}
	if things, _ := doc["things"].([]any); len(things) != 0 {
		t.Fatalf("seed things: %s", got.Body)
	}

	if _, err := b.Grant(lookup(t, b, parent), "fun", 90, "keep", "keep-look"); err != nil {
		t.Fatal(err)
	}
	st := get(t, h, parent, "/v1/status")
	statusPiles, _ := asMap(t, st.Body)["piles"].([]any)
	if len(statusPiles) != 1 || statusPiles[0].(map[string]any)["id"] != "fun" {
		t.Fatalf("status piles: %s", st.Body)
	}
	put := doJSON(t, h, http.MethodPut, parent, "/v1/look", doc, "")
	if put.StatusCode != 200 {
		t.Fatalf("put look: %d %s", put.StatusCode, put.Body)
	}
	st = get(t, h, parent, "/v1/status")
	if int(asMap(t, st.Body)["groups"].(map[string]any)["fun"].(float64)) != 3690 {
		t.Fatalf("put look wrote remaining: %s", st.Body)
	}
	again := doJSON(t, h, http.MethodPut, parent, "/v1/look", asMap(t, put.Body), "")
	if again.StatusCode != 200 {
		t.Fatalf("put same look: %d %s", again.StatusCode, again.Body)
	}
	st = get(t, h, parent, "/v1/status")
	if int(asMap(t, st.Body)["groups"].(map[string]any)["fun"].(float64)) != 3690 {
		t.Fatalf("same-bytes put look wrote remaining: %s", st.Body)
	}
}

func TestSearchAndLookThings(t *testing.T) {
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := bank.Open(filepath.Join(t.TempDir(), "ledger.sqlite"), cfg, afternoon)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	parent, _, err := b.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	readSecret, _, err := b.Mint(lookup(t, b, parent), bank.MintSpec{Name: "reader", Kind: bank.KindRead})
	if err != nil {
		t.Fatal(err)
	}
	api := New(b)
	api.SearchEnv = func() ([]look.InstalledApp, look.Focus, func(string) bool) {
		return []look.InstalledApp{{
			ID: "org.prismlauncher.PrismLauncher", Name: "Prism Launcher", WMClass: "PrismLauncher",
		}}, look.Focus{}, nil
	}
	h := httptest.NewServer(api.Handler())
	t.Cleanup(h.Close)

	if get(t, h, readSecret, "/v1/search?q=mine&list=fun").StatusCode != 403 {
		t.Fatal("read token must not search")
	}
	mine := get(t, h, parent, "/v1/search?q=mine&list=fun")
	if mine.StatusCode != 200 {
		t.Fatalf("search: %d %s", mine.StatusCode, mine.Body)
	}
	body := asMap(t, mine.Body)
	hits := body["hits"].([]any)
	if len(hits) != 1 || hits[0].(map[string]any)["name"] != "Minecraft" {
		t.Fatalf("mine hits: %s", mine.Body)
	}
	if _, ok := hits[0].(map[string]any)["class"]; ok {
		t.Fatal("class on search")
	}
	empty := get(t, h, parent, "/v1/search?q=mine&list=fun")
	api.SearchEnv = func() ([]look.InstalledApp, look.Focus, func(string) bool) {
		return nil, look.Focus{}, nil
	}
	empty = get(t, h, parent, "/v1/search?q=mine&list=fun")
	if len(asMap(t, empty.Body)["hits"].([]any)) != 0 {
		t.Fatalf("mine without prism: %s", empty.Body)
	}
	api.SearchEnv = func() ([]look.InstalledApp, look.Focus, func(string) bool) {
		return []look.InstalledApp{{
			ID: "org.prismlauncher.PrismLauncher", Name: "Prism Launcher", WMClass: "PrismLauncher",
		}}, look.Focus{}, nil
	}
	khan := get(t, h, parent, "/v1/search?q=khanacademy.org&list=fun")
	if len(asMap(t, khan.Body)["hits"].([]any)) != 0 {
		t.Fatalf("khan on fun: %s", khan.Body)
	}
	paste := get(t, h, parent, "/v1/search?q=youtube.com&list=fun")
	if asMap(t, paste.Body)["hits"].([]any)[0].(map[string]any)["id"] != "site:youtube.com" {
		t.Fatalf("youtube paste: %s", paste.Body)
	}

	got := get(t, h, parent, "/v1/look")
	doc := asMap(t, got.Body)
	doc["things"] = []any{map[string]any{"id": "desktop:org.prismlauncher.PrismLauncher", "name": "Minecraft", "kind": "app"}}
	doc["apps"] = map[string]any{"desktop:org.prismlauncher.PrismLauncher": "fun"}
	put := doJSON(t, h, http.MethodPut, parent, "/v1/look", doc, "")
	if put.StatusCode != 200 {
		t.Fatalf("put minecraft: %d %s", put.StatusCode, put.Body)
	}
	out := asMap(t, put.Body)
	if out["fun_hours"].(map[string]any)["sat"].(float64) != 7200 {
		t.Fatalf("sat survived add: %s", put.Body)
	}
	if _, ok := out["matchers"]; ok {
		t.Fatal("matchers leaked")
	}
	if raw, _ := json.Marshal(out); strings.Contains(string(raw), "(?i)") || strings.Contains(string(raw), "ClassExact") {
		t.Fatalf("matcher leaked: %s", out)
	}
}

type httpRes struct {
	StatusCode int
	Body       string
}

func start(t *testing.T) (*httptest.Server, string, *bank.Bank) {
	t.Helper()
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enforcer || cfg.BedtimeLock {
		t.Fatal("http tests must use enforcer off and bedtime_lock off")
	}
	b, err := bank.Open(filepath.Join(t.TempDir(), "ledger.sqlite"), cfg, afternoon)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	secret, _, err := b.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(New(b).Handler())
	t.Cleanup(srv.Close)
	return srv, secret, b
}

func post(t *testing.T, h *httptest.Server, token, path string, body map[string]any, idem string) httpRes {
	t.Helper()
	return doJSON(t, h, http.MethodPost, token, path, body, idem)
}

func patch(t *testing.T, h *httptest.Server, token, path string, body map[string]any) httpRes {
	t.Helper()
	return doJSON(t, h, http.MethodPatch, token, path, body, "")
}

func doJSON(t *testing.T, h *httptest.Server, method, token, path string, body map[string]any, idem string) httpRes {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, h.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return httpRes{StatusCode: resp.StatusCode, Body: string(b)}
}

func lookup(t *testing.T, b *bank.Bank, secret string) *bank.Token {
	t.Helper()
	tok, err := b.LookupSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func get(t *testing.T, h *httptest.Server, token, path string) httpRes {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, h.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return httpRes{StatusCode: resp.StatusCode, Body: string(b)}
}

func asMap(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("json %s: %v", body, err)
	}
	return m
}

func afternoon() time.Time {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return time.Date(2026, 8, 26, 15, 0, 0, 0, loc)
}

func packagingPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	return filepath.Join(root, "packaging", name)
}
