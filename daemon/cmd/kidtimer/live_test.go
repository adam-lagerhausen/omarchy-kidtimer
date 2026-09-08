package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"kidtimer/daemon/internal/config"
)

type liveDaemon struct {
	bin    string
	base   string
	addr   string
	parent string
	db     string
	cmd    *exec.Cmd
	stderr *bytes.Buffer
}

func TestLiveDaemonFeatures(t *testing.T) {
	bin := buildKidtimer(t)
	t.Run("grant", func(t *testing.T) { liveGrant(t, launchDaemon(t, bin, "", "")) })
	t.Run("status", func(t *testing.T) { liveStatus(t, launchDaemon(t, bin, "", "")) })
	t.Run("ask-decide", func(t *testing.T) { liveAskDecide(t, launchDaemon(t, bin, "", "")) })
	t.Run("lock", func(t *testing.T) { liveLock(t, launchDaemon(t, bin, "", "")) })
	t.Run("pin", func(t *testing.T) { livePin(t, launchDaemon(t, bin, "", "")) })
	t.Run("policy", func(t *testing.T) { livePolicy(t, bin) })
	t.Run("look", func(t *testing.T) { liveLook(t, launchDaemon(t, bin, "", "")) })
	t.Run("mint-token", func(t *testing.T) { liveMint(t, launchDaemon(t, bin, "", "")) })
}

func TestTwoKidsIsolated(t *testing.T) {
	bin := buildKidtimer(t)
	sam := launchDaemon(t, bin, "", "")
	alex := launchDaemon(t, bin, "", "")
	samSeed := funOf(t, statusMap(t, sam))
	alexSeed := funOf(t, statusMap(t, alex))

	lock := postJSON(t, sam.base+"/v1/lock", sam.parent, `{"locked":true}`, "")
	if lock.status != 200 {
		t.Fatalf("lock sam: %d %s", lock.status, lock.body)
	}
	if statusMap(t, sam)["parent_locked"] != true {
		t.Fatal("sam should be locked")
	}
	if statusMap(t, alex)["parent_locked"] != false {
		t.Fatal("alex must stay unlocked")
	}

	plus := postJSON(t, sam.base+"/v1/grants", sam.parent, `{"group":"fun","seconds":600,"reason":"+10"}`, "parent-plus10")
	if plus.status != 200 {
		t.Fatalf("+10 sam: %d %s", plus.status, plus.body)
	}
	if funOf(t, statusMap(t, sam)) != samSeed+600 {
		t.Fatal("sam +10")
	}
	if funOf(t, statusMap(t, alex)) != alexSeed {
		t.Fatal("alex remaining changed")
	}

	askSecret := mintKind(t, alex, "ask", "kid-bar")
	ask := postJSON(t, alex.base+"/v1/asks", askSecret, `{"group":"fun","seconds":900,"reason":"one more video"}`, "")
	if ask.status != 200 {
		t.Fatalf("alex ask: %d %s", ask.status, ask.body)
	}
	askID := asMap(t, ask.body)["id"].(string)
	decide := postJSON(t, alex.base+"/v1/asks/"+askID+"/decide", alex.parent, `{"decision":"approve"}`, "")
	if decide.status != 200 {
		t.Fatalf("approve alex: %d %s", decide.status, decide.body)
	}
	if funOf(t, statusMap(t, alex)) != alexSeed+900 {
		t.Fatal("alex approve")
	}
	if funOf(t, statusMap(t, sam)) != samSeed+600 {
		t.Fatal("sam changed when alex ask was approved")
	}

	if err := alex.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_, _ = alex.cmd.Process.Wait()
	down := getJSON(t, alex.base+"/v1/status", alex.parent)
	if down.status == 200 {
		t.Fatal("alex still reachable")
	}
	if getJSON(t, sam.base+"/v1/status", sam.parent).status != 200 {
		t.Fatal("sam poll must survive alex going down")
	}
}

func liveGrant(t *testing.T, d *liveDaemon) {
	t.Helper()
	seed := funOf(t, statusMap(t, d))
	minus := postJSON(t, d.base+"/v1/grants", d.parent, `{"group":"fun","seconds":-600,"reason":"-10"}`, "parent-minus10")
	if minus.status != 200 {
		t.Fatalf("-10: %d %s", minus.status, minus.body)
	}
	body := asMap(t, minus.body)
	if int(body["seconds"].(float64)) != -600 || body["reason"] != "-10" || body["source"] != "parent" {
		t.Fatalf("-10 body: %s", minus.body)
	}
	if funOf(t, statusMap(t, d)) != seed-600 {
		t.Fatal("status after -10")
	}
	replayMinus := postJSON(t, d.base+"/v1/grants", d.parent, `{"group":"fun","seconds":-600,"reason":"-10"}`, "parent-minus10")
	if asMap(t, replayMinus.body)["replay"] != true || funOf(t, statusMap(t, d)) != seed-600 {
		t.Fatalf("replay -10: %s", replayMinus.body)
	}

	grantOut := runScript(t, "grant.sh", d.base, d.parent, nil)
	first := asMap(t, grantOut)
	if first["group"] != "fun" || int(first["seconds"].(float64)) != 900 || first["reason"] != "good afternoon" {
		t.Fatalf("grant.sh: %s", grantOut)
	}
	if first["replay"] == true {
		t.Fatal("first grant.sh must not be a replay")
	}
	if int(first["remaining"].(float64)) != seed-600+900 {
		t.Fatalf("fun after grant.sh: %s", grantOut)
	}
	replayOut := runScript(t, "grant.sh", d.base, d.parent, nil)
	if asMap(t, replayOut)["replay"] != true {
		t.Fatalf("replay grant.sh: %s", replayOut)
	}
	conflict := postJSON(t, d.base+"/v1/grants", d.parent, `{"group":"fun","seconds":1,"reason":"other"}`, "testdata-grant-1")
	if conflict.status != 409 {
		t.Fatalf("same key different body: %d %s", conflict.status, conflict.body)
	}

	plus := postJSON(t, d.base+"/v1/grants", d.parent, `{"group":"fun","seconds":600,"reason":"+10"}`, "parent-plus10")
	if plus.status != 200 || asMap(t, plus.body)["reason"] != "+10" {
		t.Fatalf("+10: %d %s", plus.status, plus.body)
	}

	cliGrant := runCLI(t, d, "grant",
		"-url", d.base,
		"-token", d.parent,
		"-seconds", "15",
		"-reason", "+15",
		"-idempotency-key", "cli-plus15",
	)
	cli := asMap(t, cliGrant)
	if cli["group"] != "fun" || int(cli["seconds"].(float64)) != 15 {
		t.Fatalf("+15 must default to fun: %s", cliGrant)
	}
	if int(cli["remaining"].(float64)) != seed-600+900+600+15 {
		t.Fatalf("fun after cli +15: %s", cliGrant)
	}
}

func liveStatus(t *testing.T, d *liveDaemon) {
	t.Helper()
	stOut := runScript(t, "status.sh", d.base, d.parent, nil)
	st := asMap(t, stOut)
	if st["kid_name"] != "parent-lab" {
		t.Fatalf("kid_name: %s", stOut)
	}
	groups := st["groups"].(map[string]any)
	if _, ok := groups["minecraft"]; ok {
		t.Fatal("minecraft clock")
	}
	if _, ok := groups["youtube"]; ok {
		t.Fatal("youtube clock")
	}
	if _, ok := groups["school"]; ok {
		t.Fatal("school clock")
	}
	if _, ok := groups["fun"]; !ok {
		t.Fatalf("fun missing: %s", stOut)
	}
	if _, ok := st["bedtime_in"]; ok {
		t.Fatal("parent-lab bedtime_lock off must omit bedtime_in")
	}
	if st["parent_locked"] != false || st["remote_lock"] != false {
		t.Fatalf("lock flags: %s", stOut)
	}
	if st["bedtime_start"] != "21:00" || st["bedtime_end"] != "07:00" {
		t.Fatalf("bedtime: %s", stOut)
	}

	cliStatus := runCLI(t, d, "status", "-url", d.base, "-token", d.parent)
	if asMap(t, cliStatus)["kid_name"] != "parent-lab" {
		t.Fatalf("cli status: %s", cliStatus)
	}

	readSecret := mintKind(t, d, "read", "bar-read")
	kidStatus := getJSON(t, d.base+"/v1/status", readSecret)
	if kidStatus.status != 200 {
		t.Fatalf("kid read status: %d %s", kidStatus.status, kidStatus.body)
	}
}

func liveAskDecide(t *testing.T, d *liveDaemon) {
	t.Helper()
	seed := funOf(t, statusMap(t, d))
	askSecret := mintKind(t, d, "ask", "kid-bar")
	askOut := runScript(t, "ask.sh", d.base, askSecret, nil)
	ask := asMap(t, askOut)
	askID, _ := ask["id"].(string)
	if askID == "" || ask["group"] != "fun" {
		t.Fatalf("ask.sh: %s", askOut)
	}
	listed := getJSON(t, d.base+"/v1/asks", d.parent)
	if listed.status != 200 {
		t.Fatalf("GET /v1/asks: %d %s", listed.status, listed.body)
	}
	if len(asMap(t, listed.body)["asks"].([]any)) != 1 {
		t.Fatalf("pending asks: %s", listed.body)
	}
	cliAsks := runCLI(t, d, "asks", "-url", d.base, "-token", d.parent)
	if len(asMap(t, cliAsks)["asks"].([]any)) != 1 {
		t.Fatalf("cli asks: %s", cliAsks)
	}
	decideOut := runScript(t, "decide.sh", d.base, d.parent, []string{askID, "approve"})
	if asMap(t, decideOut)["status"] != "approved" {
		t.Fatalf("decide.sh: %s", decideOut)
	}
	if funOf(t, statusMap(t, d)) != seed+900 {
		t.Fatal("approve did not credit fun")
	}
	again := postJSON(t, d.base+"/v1/asks/"+askID+"/decide", d.parent, `{"decision":"approve"}`, "")
	if again.status != 409 || !strings.Contains(again.body, "already decided") {
		t.Fatalf("second decide: %d %s", again.status, again.body)
	}

	denyAsk := postJSON(t, d.base+"/v1/asks", askSecret, `{"group":"fun","seconds":60,"reason":"more time"}`, "")
	if denyAsk.status != 200 {
		t.Fatalf("kid panel ask: %d %s", denyAsk.status, denyAsk.body)
	}
	denyID := asMap(t, denyAsk.body)["id"].(string)
	denied := postJSON(t, d.base+"/v1/asks/"+denyID+"/decide", d.parent, `{"decision":"deny"}`, "")
	if denied.status != 200 || asMap(t, denied.body)["status"] != "denied" {
		t.Fatalf("parent deny: %d %s", denied.status, denied.body)
	}
	if funOf(t, statusMap(t, d)) != seed+900 {
		t.Fatal("deny credited")
	}

	cliAsk := postJSON(t, d.base+"/v1/asks", askSecret, `{"group":"fun","seconds":30,"reason":"cli"}`, "")
	if cliAsk.status != 200 {
		t.Fatalf("cli ask: %d %s", cliAsk.status, cliAsk.body)
	}
	cliID := asMap(t, cliAsk.body)["id"].(string)
	cliDecide := runCLI(t, d, "decide", "-url", d.base, "-token", d.parent, cliID, "approve")
	if asMap(t, cliDecide)["status"] != "approved" {
		t.Fatalf("cli decide: %s", cliDecide)
	}
	if funOf(t, statusMap(t, d)) != seed+930 {
		t.Fatal("cli decide did not credit")
	}
}

func livePin(t *testing.T, d *liveDaemon) {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command(d.bin, "pin", "set", "-home", home, "-desk", "", "-url", d.base, "-token", d.parent)
	cmd.Stdin = strings.NewReader("4242\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pin set: %v %s", err, out)
	}
	st := statusMap(t, d)
	if st["parent_pin_set"] != true {
		t.Fatalf("status after pin set: %v", st)
	}
	askSecret := mintKind(t, d, "ask", "kid-bar")
	putReq, err := http.NewRequest(http.MethodPut, d.base+"/v1/parent-pin", strings.NewReader(`{"pin":"9999"}`))
	if err != nil {
		t.Fatal(err)
	}
	putReq.Header.Set("Authorization", "Bearer "+askSecret)
	putReq.Header.Set("Content-Type", "application/json")
	if doJSON(t, putReq).status != 403 {
		t.Fatal("ask must not set pin")
	}
	ask := postJSON(t, d.base+"/v1/asks", askSecret, `{"group":"fun","seconds":120,"reason":"more time"}`, "")
	if ask.status != 200 {
		t.Fatalf("ask: %d %s", ask.status, ask.body)
	}
	askID := asMap(t, ask.body)["id"].(string)
	if postJSON(t, d.base+"/v1/pin/approve", askSecret, `{"pin":"0000","ask_id":"`+askID+`"}`, "").status != 403 {
		t.Fatal("wrong pin must 403")
	}
	ok := postJSON(t, d.base+"/v1/pin/approve", askSecret, `{"pin":"4242","ask_id":"`+askID+`"}`, "")
	if ok.status != 200 || asMap(t, ok.body)["status"] != "approved" {
		t.Fatalf("pin approve: %d %s", ok.status, ok.body)
	}
	grant := postJSON(t, d.base+"/v1/pin/grant", askSecret, `{"pin":"4242","seconds":60}`, "")
	if grant.status != 200 || asMap(t, grant.body)["source"] != "parent-pin" {
		t.Fatalf("pin grant: %d %s", grant.status, grant.body)
	}
}

func liveLock(t *testing.T, d *liveDaemon) {
	t.Helper()
	lockOut := runScript(t, "lock.sh", d.base, d.parent, nil)
	if !asMap(t, lockOut)["locked"].(bool) {
		t.Fatalf("lock.sh: %s", lockOut)
	}
	if statusMap(t, d)["parent_locked"] != true {
		t.Fatal("status after lock")
	}
	replay := runScript(t, "lock.sh", d.base, d.parent, nil)
	if !asMap(t, replay)["locked"].(bool) {
		t.Fatalf("lock replay: %s", replay)
	}
	unlock := postJSON(t, d.base+"/v1/lock", d.parent, `{"locked":false}`, "")
	if unlock.status != 200 || asMap(t, unlock.body)["locked"] != false {
		t.Fatalf("unlock: %d %s", unlock.status, unlock.body)
	}
	lockCLI := runCLI(t, d, "lock", "-url", d.base, "-token", d.parent)
	if !asMap(t, lockCLI)["locked"].(bool) {
		t.Fatalf("cli lock: %s", lockCLI)
	}
	if statusMap(t, d)["parent_locked"] != true {
		t.Fatal("status after cli lock")
	}
	unlockCLI := runCLI(t, d, "unlock", "-url", d.base, "-token", d.parent)
	if asMap(t, unlockCLI)["locked"] != false {
		t.Fatalf("cli unlock: %s", unlockCLI)
	}
	if statusMap(t, d)["parent_locked"] != false {
		t.Fatal("status after cli unlock")
	}
	askSecret := mintKind(t, d, "ask", "kid-bar")
	if postJSON(t, d.base+"/v1/lock", askSecret, `{"locked":true}`, "").status != 403 {
		t.Fatal("ask must not lock")
	}
}

func livePolicy(t *testing.T, bin string) {
	t.Helper()
	db := filepath.Join(t.TempDir(), "ledger.sqlite")
	d := launchDaemon(t, bin, db, "")
	seed := funOf(t, statusMap(t, d))
	out := runScript(t, "policy.sh", d.base, d.parent, nil)
	pol := asMap(t, out)
	if pol["bedtime_start"] != "20:00" {
		t.Fatalf("policy.sh: %s", out)
	}
	if funOf(t, pol) != seed {
		t.Fatalf("policy refilled: %s", out)
	}
	if err := d.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	_, _ = d.cmd.Process.Wait()
	again := launchDaemon(t, bin, db, d.parent)
	st := statusMap(t, again)
	if st["bedtime_start"] != "20:00" || st["bedtime_end"] != "07:00" {
		t.Fatalf("policy persist: %v", st)
	}
	if funOf(t, st) != seed {
		t.Fatal("remaining after relaunch")
	}
}

func liveLook(t *testing.T, d *liveDaemon) {
	t.Helper()
	seed := funOf(t, statusMap(t, d))
	got := getJSON(t, d.base+"/v1/look", d.parent)
	if got.status != 200 {
		t.Fatalf("get look: %d %s", got.status, got.body)
	}
	doc := asMap(t, got.body)
	if _, ok := doc["catalog"]; ok {
		t.Fatal("catalog on look wire")
	}
	plus := postJSON(t, d.base+"/v1/grants", d.parent, `{"group":"fun","seconds":90,"reason":"keep"}`, "keep-look")
	if plus.status != 200 {
		t.Fatalf("grant: %d %s", plus.status, plus.body)
	}
	putReq, err := http.NewRequest(http.MethodPut, d.base+"/v1/look", strings.NewReader(got.body))
	if err != nil {
		t.Fatal(err)
	}
	putReq.Header.Set("Authorization", "Bearer "+d.parent)
	putReq.Header.Set("Content-Type", "application/json")
	put := doJSON(t, putReq)
	if put.status != 200 {
		t.Fatalf("put look: %d %s", put.status, put.body)
	}
	if funOf(t, statusMap(t, d)) != seed+90 {
		t.Fatal("put look wrote remaining")
	}
}

func liveMint(t *testing.T, d *liveDaemon) {
	t.Helper()
	appOut := runScript(t, "token-create.sh", d.base, d.parent, nil)
	app := asMap(t, appOut)
	if app["kind"] != "app" || app["groups"].([]any)[0] != "fun" {
		t.Fatalf("token-create.sh: %s", appOut)
	}
	if app["secret"] == "" {
		t.Fatal("app secret shown once")
	}
	askOut, err := exec.Command(d.bin, "token", "create",
		"-url", d.base, "-token", d.parent, "-name", "kid-bar", "-kind", "ask",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("cli token create ask: %v\n%s", err, askOut)
	}
	if asMap(t, string(askOut))["secret"] == "" {
		t.Fatal("ask secret shown once")
	}
	readOut, err := exec.Command(d.bin, "token", "create",
		"-url", d.base, "-token", d.parent, "-name", "bar-read", "-kind", "read",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("cli token create read: %v\n%s", err, readOut)
	}
	if asMap(t, string(readOut))["secret"] == "" {
		t.Fatal("read secret shown once")
	}
}

func launchDaemon(t *testing.T, bin, db, parent string) *liveDaemon {
	t.Helper()
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enforcer || cfg.BedtimeLock {
		t.Fatal("live tests must use parent-lab enforcer off and bedtime_lock off")
	}
	addr := freeAddr(t)
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("listen host %s", host)
	}
	if port == "8742" {
		t.Fatal("must not bind 8742")
	}
	if db == "" {
		db = filepath.Join(t.TempDir(), "ledger.sqlite")
	}
	if !strings.HasPrefix(db, os.TempDir()) && !strings.Contains(db, t.TempDir()) {
		if !strings.Contains(filepath.Clean(db), "Temp") && !strings.Contains(db, "/tmp/") {
			t.Fatalf("sqlite must be disposable: %s", db)
		}
	}
	var stderr bytes.Buffer
	cmd := exec.Command(bin, "daemon",
		"-config", packagingPath(t, "config.parent-lab.toml"),
		"-data", db,
		"-listen", addr,
	)
	cmd.Stderr = &stderr
	cmd.Stdout = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_, _ = cmd.Process.Wait()
	})
	base := "http://" + addr
	waitReady(t, base, &stderr)
	if parent == "" {
		parent = bootstrapToken(t, stderr.String(), &stderr)
	}
	if getJSON(t, base+"/v1/status", "").status != 401 {
		t.Fatal("unauthenticated status must be 401")
	}
	return &liveDaemon{bin: bin, base: base, addr: addr, parent: parent, db: db, cmd: cmd, stderr: &stderr}
}

func runCLI(t *testing.T, d *liveDaemon, args ...string) string {
	t.Helper()
	cmd := exec.Command(d.bin, args...)
	cmd.Env = cliTestEnv(d.base, d.parent)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cli %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func cliTestEnv(url, token string) []string {
	var env []string
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "KIDTIMER_URL=") || strings.HasPrefix(e, "KIDTIMER_TOKEN=") {
			continue
		}
		env = append(env, e)
	}
	return append(env, "KIDTIMER_URL="+url, "KIDTIMER_TOKEN="+token)
}

func mintKind(t *testing.T, d *liveDaemon, kind, name string) string {
	t.Helper()
	out, err := exec.Command(d.bin, "token", "create",
		"-url", d.base, "-token", d.parent, "-name", name, "-kind", kind,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("token create %s: %v\n%s", kind, err, out)
	}
	secret, _ := asMap(t, string(out))["secret"].(string)
	if secret == "" {
		t.Fatalf("missing secret: %s", out)
	}
	return secret
}

func statusMap(t *testing.T, d *liveDaemon) map[string]any {
	t.Helper()
	res := getJSON(t, d.base+"/v1/status", d.parent)
	if res.status != 200 {
		t.Fatalf("status: %d %s", res.status, res.body)
	}
	return asMap(t, res.body)
}

func funOf(t *testing.T, st map[string]any) int {
	t.Helper()
	groups, _ := st["groups"].(map[string]any)
	n, ok := groups["fun"].(float64)
	if !ok {
		t.Fatalf("groups.fun: %v", st["groups"])
	}
	return int(n)
}

func buildKidtimer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "kidtimer")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = filepath.Dir(thisFile())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func waitReady(t *testing.T, base string, stderr *bytes.Buffer) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, base+"/v1/status", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("daemon not ready\n%s", stderr.String())
}

func bootstrapToken(t *testing.T, log string, stderr *bytes.Buffer) string {
	t.Helper()
	re := regexp.MustCompile(`bootstrap parent token \S+ \(shown once\): (\S+)`)
	m := re.FindStringSubmatch(log)
	if len(m) != 2 {
		t.Fatalf("bootstrap token missing\n%s", stderr.String())
	}
	return m[1]
}

func runScript(t *testing.T, name, base, token string, args []string) string {
	t.Helper()
	script := filepath.Join(repoRoot(t), "testdata", name)
	cmd := exec.Command(script, args...)
	cmd.Env = append(os.Environ(),
		"KIDTIMER_URL="+base,
		"KIDTIMER_TOKEN="+token,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", name, err, out)
	}
	return string(out)
}

type httpRes struct {
	status int
	body   string
}

func postJSON(t *testing.T, url, token, body, idem string) httpRes {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	return doJSON(t, req)
}

func getJSON(t *testing.T, url, token string) httpRes {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return doJSON(t, req)
}

func doJSON(t *testing.T, req *http.Request) httpRes {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return httpRes{status: 0, body: err.Error()}
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return httpRes{status: resp.StatusCode, body: string(b)}
}

func asMap(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &m); err != nil {
		t.Fatalf("json %s: %v", body, err)
	}
	return m
}

func packagingPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "packaging", name)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(filepath.Dir(thisFile()), "..", "..", "..")
}

func thisFile() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("caller")
	}
	return file
}

func TestFreeAddrIsLocalhost(t *testing.T) {
	host, port, err := net.SplitHostPort(freeAddr(t))
	if err != nil {
		t.Fatal(err)
	}
	if host != "127.0.0.1" {
		t.Fatalf("host %s", host)
	}
	if _, err := strconv.Atoi(port); err != nil {
		t.Fatal(err)
	}
}

func TestDaemonListenDefaultIsLocalhost(t *testing.T) {
	src, err := os.ReadFile(thisFile()[:len(thisFile())-len("live_test.go")] + "main.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "netaddr.DefaultListen") {
		t.Fatal("daemon listen default must be 127.0.0.1:8742")
	}
}
