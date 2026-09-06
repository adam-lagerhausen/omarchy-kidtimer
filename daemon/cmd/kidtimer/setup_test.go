package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

func TestSetupParentLeavesBankAlone(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	if err := setupParent(env); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "etc", "kidtimer")); err == nil {
		t.Fatal("parent setup wrote /etc/kidtimer")
	}
	if _, err := os.Stat(filepath.Join(root, "usr", "local", "bin", "kidtimer")); err == nil {
		t.Fatal("parent setup wrote system binary")
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".local", "bin", "kidtimer")); err != nil {
		t.Fatal("parent binary")
	}
	dest, err := os.Readlink(filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer.parent"))
	if err != nil || !strings.Contains(dest, "plugin-parent") {
		t.Fatalf("plugin %s %v", dest, err)
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer.kid")); err == nil {
		t.Fatal("parent setup must not install kid plugin")
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".local", "share", "kidtimer")); err != nil {
		t.Fatal("household dir")
	}
	role, err := household.LoadRole(filepath.Join(env.Home, ".local", "share", "kidtimer"))
	if err != nil || role != reverse.RoleParent {
		t.Fatalf("parent role %v %v", role, err)
	}
	ids := widgetIDs(t, filepath.Join(env.Home, ".config", "omarchy", "shell.json"))
	if strings.Contains(ids, "kidtimer.kid") || !strings.Contains(ids, "kidtimer.parent") {
		t.Fatalf("widgets %s", ids)
	}
	if !strings.Contains(ids, filepath.Join(env.Home, ".local", "bin", "kidtimer")) {
		t.Fatalf("parentBin %s", ids)
	}
}

func TestSetupParentImportsLegacyAllowance(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	legacy := filepath.Join(env.Home, ".local", "share", "allowance")
	if err := household.Save(household.Path(legacy), []reverse.Record{{
		ID: "kid-1", Name: "testMax", URL: "http://100.64.1.2:8742", Token: "secret",
	}}); err != nil {
		t.Fatal(err)
	}
	if err := setupParent(env); err != nil {
		t.Fatal(err)
	}
	got, err := household.Load(household.Path(filepath.Join(env.Home, ".local", "share", "kidtimer")))
	if err != nil || len(got) != 1 || got[0].Name != "testMax" || got[0].Token != "secret" {
		t.Fatalf("imported %+v %v", got, err)
	}
}

func TestSetupParentRemovesAllowanceChip(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	plug := filepath.Join(env.Home, ".config", "omarchy", "plugins", "allowance.parent")
	if err := os.MkdirAll(filepath.Dir(plug), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/tmp/allowance-parent", plug); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	if err := ensureWidget(shell, "allowance.parent", map[string]any{"parentBin": "old"}); err != nil {
		t.Fatal(err)
	}
	if err := setupParent(env); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(plug); !os.IsNotExist(err) {
		t.Fatalf("plugin leftover %v", err)
	}
	ids := widgetIDs(t, shell)
	if strings.Contains(ids, "allowance.parent") {
		t.Fatalf("widget leftover %s", ids)
	}
	if !strings.Contains(ids, "kidtimer.parent") {
		t.Fatalf("kidtimer widget %s", ids)
	}
}

func TestSetupKidMintsBarTokens(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "usr", "local", "bin", "kidtimer")); err != nil {
		t.Fatal("kid binary")
	}
	if _, err := os.Stat(filepath.Join(root, "etc", "kidtimer", "config.toml")); err != nil {
		t.Fatal("kid config")
	}
	if _, err := os.Stat(filepath.Join(root, "var", "lib", "kidtimer", "ledger.sqlite")); err != nil {
		t.Fatal("ledger")
	}
	if _, err := os.Stat(filepath.Join(root, "var", "lib", "kidtimer", "machine-id")); err != nil {
		t.Fatal("machine-id")
	}
	if _, err := os.Stat(filepath.Join(root, "etc", "systemd", "system", "kidtimer.service")); err != nil {
		t.Fatal("unit")
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer.parent")); err == nil {
		t.Fatal("kid setup must not install parent plugin")
	}
	raw, err := os.ReadFile(filepath.Join(env.Home, ".config", "omarchy", "shell.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var kid map[string]any
	for _, w := range widgets(doc) {
		if str(w["id"]) == "kidtimer.kid" {
			kid = w
		}
	}
	if kid == nil || str(kid["readToken"]) == "" || str(kid["askToken"]) == "" {
		t.Fatalf("kid widget: %s", raw)
	}
	if str(kid["kidBin"]) != filepath.Join(root, "usr", "local", "bin", "kidtimer") {
		t.Fatalf("kidBin %v", kid["kidBin"])
	}
	if str(kid["url"]) != "http://127.0.0.1:8742" {
		t.Fatalf("url %v", kid["url"])
	}
	cfg, err := os.ReadFile(filepath.Join(root, "etc", "kidtimer", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), "advertise = true") {
		t.Fatal("advertise")
	}
	if strings.Contains(string(cfg), `kid_name = "kid-a"`) {
		t.Fatal("setup should replace packaged kid_name")
	}
}

func TestSetupKidTwice(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, "etc", "kidtimer", "config.toml")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	custom := strings.Replace(string(raw), `listen = "0.0.0.0:8742"`, `listen = "127.0.0.1:8742"`, 1)
	if custom == string(raw) {
		t.Fatal("need packaged listen to customize")
	}
	if err := os.WriteFile(cfgPath, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `listen = "0.0.0.0:8742"`) || !strings.Contains(string(got), "advertise = true") {
		t.Fatalf("localhost workaround must become LAN: %s", got)
	}
}

func TestSetupKidKeepsOtherKidEdits(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, "etc", "kidtimer", "config.toml")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	custom := strings.Replace(string(raw), `bedtime_start = "21:00"`, `bedtime_start = "20:00"`, 1)
	if custom == string(raw) {
		t.Fatal("need packaged bedtime to customize")
	}
	if err := os.WriteFile(cfgPath, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(cfgPath)
	if err != nil || !strings.Contains(string(got), `bedtime_start = "20:00"`) {
		t.Fatalf("other edits must stay: %s %v", got, err)
	}
	if !strings.Contains(string(got), `listen = "0.0.0.0:8742"`) {
		t.Fatalf("lan listen: %s", got)
	}
}

func TestSetupKidRepairsHalfway(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, "etc", "kidtimer", "config.toml")
	if err := os.WriteFile(cfgPath, []byte("not toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	if err := os.WriteFile(shell, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer.kid")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "etc", "systemd", "system", "kidtimer.service")); err != nil {
		t.Fatal(err)
	}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	if _, err := config.ParseFile(cfgPath); err != nil {
		t.Fatalf("repaired config: %v", err)
	}
	doc, err := readJSON(shell)
	if err != nil {
		t.Fatal(err)
	}
	var kid map[string]any
	for _, w := range widgets(doc) {
		if str(w["id"]) == "kidtimer.kid" {
			kid = w
		}
	}
	if kid == nil || str(kid["readToken"]) == "" {
		t.Fatalf("repaired widget: %v", doc)
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer.kid")); err != nil {
		t.Fatal("repaired plugin")
	}
	if _, err := os.Stat(filepath.Join(root, "etc", "systemd", "system", "kidtimer.service")); err != nil {
		t.Fatal("repaired unit")
	}
}

func TestSetupKidRepairsStaleTokens(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	if err := ensureWidget(shell, "kidtimer.kid", map[string]any{
		"url": "http://127.0.0.1:8742", "readToken": "dead", "askToken": "dead",
	}); err != nil {
		t.Fatal(err)
	}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	doc, err := readJSON(shell)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range widgets(doc) {
		if str(w["id"]) != "kidtimer.kid" {
			continue
		}
		if str(w["readToken"]) == "dead" || str(w["askToken"]) == "dead" {
			t.Fatal("stale tokens must be reminted")
		}
		if str(w["readToken"]) == "" || str(w["askToken"]) == "" {
			t.Fatal("missing remint")
		}
		return
	}
	t.Fatal("kid widget")
}

func TestMintKidBarAfterPartialSetup(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	if err := os.Remove(shell); err != nil {
		t.Fatal(err)
	}
	readTok, askTok, err := mintKidBar(filepath.Join(root, "var", "lib", "kidtimer", "ledger.sqlite"), filepath.Join(root, "etc", "kidtimer", "config.toml"), env.Home)
	if err != nil || readTok == "" || askTok == "" {
		t.Fatalf("remint: %v %s %s", err, readTok, askTok)
	}
}

func TestHomeFromPkexec(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	t.Setenv("PKEXEC_UID", strconv.Itoa(os.Getuid()))
	got := homeFromEnv()
	want, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("pkexec home %q want %q", got, want)
	}
}

func TestCopyFileDoesNotTruncateInPlace(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.WriteFile(src, []byte("hello-kidtimer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "hello-kidtimer" {
		t.Fatalf("dst %s %v", got, err)
	}
	if err := copyFile(dst, dst, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestUserDaemonOwnsLedger(t *testing.T) {
	ledger := "/home/test/.local/share/kidtimer/ledger.sqlite"
	user := "/home/test/.local/bin/kidtimer\x00daemon\x00-config\x00/home/test/.local/share/kidtimer/config.kid.toml\x00-data\x00/home/test/.local/share/kidtimer/ledger.sqlite\x00-listen\x00127.0.0.1:8742\x00"
	if !userDaemonOwnsLedger(user, ledger) {
		t.Fatal("user leftover daemon")
	}
	sys := "/usr/local/bin/kidtimer\x00daemon\x00"
	if userDaemonOwnsLedger(sys, ledger) {
		t.Fatal("systemd daemon")
	}
	kid := "/home/test/.local/bin/kidtimer\x00kid\x00"
	if userDaemonOwnsLedger(kid, ledger) {
		t.Fatal("kidtimer kid")
	}
	other := "/home/test/.local/bin/kidtimer\x00daemon\x00-data\x00/home/other/.local/share/kidtimer/ledger.sqlite\x00"
	if userDaemonOwnsLedger(other, ledger) {
		t.Fatal("other user's ledger")
	}
}

func TestUserBankPIDPath(t *testing.T) {
	got := userBankPIDPath("/home/test/.local/share/kidtimer/ledger.sqlite")
	want := "/home/test/.local/share/kidtimer/daemon.pid"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if p := userBankPIDPath("/var/lib/kidtimer/ledger.sqlite"); p != "" {
		t.Fatalf("system ledger wrote pid %q", p)
	}
	if p := userBankPIDPath("/home/parent/.local/share/kidtimer-lab/ledger.sqlite"); p != "" {
		t.Fatalf("lab ledger wrote pid %q", p)
	}
	if p := userBankPIDPath("/tmp/kidtimer-verify-x/ledger.sqlite"); p != "" {
		t.Fatalf("verify ledger wrote pid %q", p)
	}
}

func TestClaimUserBankPID(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".local", "share", "kidtimer")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "ledger.sqlite")
	release := claimUserBankPID(db)
	raw, err := os.ReadFile(filepath.Join(dir, "daemon.pid"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(raw)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("pid file %q", raw)
	}
	release()
	if _, err := os.Stat(filepath.Join(dir, "daemon.pid")); !os.IsNotExist(err) {
		t.Fatalf("pid file should be gone: %v", err)
	}
}

func TestInstallScriptRequiresRole(t *testing.T) {
	script := filepath.Join(repoRoot(t), "packaging", "install.sh")
	out, err := exec.Command("bash", script).CombinedOutput()
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(string(out), "parent|kid") {
		t.Fatalf("usage: %s", out)
	}
}

func TestInstallScriptParentHasNoSudo(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "packaging", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "setup parent") {
		t.Fatal("missing setup parent")
	}
	if strings.Count(body, "sudo") != 1 {
		t.Fatalf("sudo should only re-exec kid, got %d", strings.Count(body, "sudo"))
	}
	if !strings.Contains(body, `exec sudo "$0" kid`) {
		t.Fatal("kid must re-exec sudo")
	}
	if !strings.Contains(body, `HOME}/.local/share/kidtimer/src`) {
		t.Fatal("parent share dir")
	}
	if !strings.Contains(body, "/usr/local/share/kidtimer") {
		t.Fatal("kid share dir")
	}
}

func TestBootstrapScriptDoesNotGuessRole(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "/dev/tty") {
		t.Fatal("must ask on /dev/tty")
	}
	if !strings.Contains(body, "releases/download/latest") {
		t.Fatal("must fetch the latest pack")
	}
	if !strings.Contains(body, "SHA256SUMS") {
		t.Fatal("must check SHA256SUMS")
	}
	if strings.Contains(body, `exec sudo "$0"`) {
		t.Fatal("curl pipe cannot re-exec $0")
	}
	if !strings.Contains(body, `sudo bash "$dir/install.sh" kid`) {
		t.Fatal("kid must sudo the unpacked install.sh")
	}
	if strings.Count(body, "sudo") != 1 {
		t.Fatalf("sudo only for kid, got %d", strings.Count(body, "sudo"))
	}
	script := filepath.Join(repoRoot(t), "install.sh")
	out, err := exec.Command("bash", script, "nope").CombinedOutput()
	if err == nil {
		t.Fatal("expected usage error")
	}
	if !strings.Contains(string(out), "parent or kid") {
		t.Fatalf("usage: %s", out)
	}
}

func TestPackScriptWritesSums(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "packaging", "pack.sh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "SHA256SUMS") {
		t.Fatal("pack must write SHA256SUMS")
	}
	if !strings.Contains(body, "sha256sum kidtimer-linux-amd64.tar.gz kidtimer-linux-arm64.tar.gz") {
		t.Fatal("pack must hash both tarballs")
	}
}

func TestBootstrapFetchesPack(t *testing.T) {
	arch := bootstrapArch(t)
	dist := t.TempDir()
	packName := "kidtimer-linux-" + arch
	packDir := filepath.Join(dist, packName)
	if err := os.Mkdir(packDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(t.TempDir(), "ran")
	stub := fmt.Sprintf("#!/bin/bash\nprintf '%%s\\n' \"$1\" > %s\n", strconv.Quote(marker))
	if err := os.WriteFile(filepath.Join(packDir, "install.sh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	tarPath := filepath.Join(dist, packName+".tar.gz")
	tar := exec.Command("tar", "-C", dist, "-czf", tarPath, packName)
	if out, err := tar.CombinedOutput(); err != nil {
		t.Fatalf("tar: %v %s", err, out)
	}
	sum := exec.Command("sha256sum", packName+".tar.gz")
	sum.Dir = dist
	sumOut, err := sum.CombinedOutput()
	if err != nil {
		t.Fatalf("sha256sum: %v %s", err, sumOut)
	}
	if err := os.WriteFile(filepath.Join(dist, "SHA256SUMS"), sumOut, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.FileServer(http.Dir(dist)))
	t.Cleanup(srv.Close)

	script := filepath.Join(repoRoot(t), "install.sh")
	cmd := exec.Command("bash", script, "parent")
	cmd.Env = append(os.Environ(), "KIDTIMER_PACK_URL="+srv.URL+"/"+packName+".tar.gz")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install: %v %s", err, out)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("inner install did not run: %v %s", err, out)
	}
	if strings.TrimSpace(string(got)) != "parent" {
		t.Fatalf("role %q", got)
	}
}

func bootstrapArch(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("uname", "-m").Output()
	if err != nil {
		t.Fatal(err)
	}
	switch strings.TrimSpace(string(out)) {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		t.Fatalf("arch %s", out)
		return ""
	}
}

func widgetIDs(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
