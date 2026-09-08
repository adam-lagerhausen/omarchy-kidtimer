package main

import (
	"encoding/json"
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
	plug := filepath.Join(env.Home, ".config", "omarchy", "plugins", pluginID, "manifest.json")
	if _, err := os.Stat(plug); err != nil {
		if dest, lerr := os.Readlink(filepath.Join(env.Home, ".config", "omarchy", "plugins", pluginID)); lerr != nil {
			t.Fatalf("plugin %v %v", err, lerr)
		} else if dest == "" {
			t.Fatalf("plugin dest %s", dest)
		}
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer.kid")); err == nil {
		t.Fatal("parent setup must not install kid plugin")
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer.parent")); err == nil {
		t.Fatal("parent setup must not leave old parent plugin")
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".local", "share", "kidtimer")); err != nil {
		t.Fatal("household dir")
	}
	role, err := household.LoadRole(filepath.Join(env.Home, ".local", "share", "kidtimer"))
	if err != nil || role != reverse.RoleParent {
		t.Fatalf("parent role %v %v", role, err)
	}
	ids := widgetIDs(t, filepath.Join(env.Home, ".config", "omarchy", "shell.json"))
	if strings.Contains(ids, `"kidtimer.kid"`) || strings.Contains(ids, `"kidtimer.parent"`) || strings.Contains(ids, `"kidtimer"`) {
		t.Fatalf("legacy widgets %s", ids)
	}
	if !strings.Contains(ids, `"`+pluginID+`"`) {
		t.Fatalf("widgets %s", ids)
	}
	if strings.Contains(ids, "parentBin") {
		t.Fatalf("parentBin must not be written: %s", ids)
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
	if !strings.Contains(ids, pluginID) {
		t.Fatalf("kidtimer widget %s", ids)
	}
}

func TestSetupParentRefusesKidComputer(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	err := setupParent(env)
	if err == nil {
		t.Fatal("parent setup must refuse a kid computer")
	}
	if !strings.Contains(err.Error(), "uninstall") {
		t.Fatalf("want uninstall: %v", err)
	}
	role, rerr := household.LoadRole(filepath.Join(env.Home, ".local", "share", "kidtimer"))
	if rerr != nil || role != reverse.RoleKid {
		t.Fatalf("kid role kept %v %v", role, rerr)
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
	if _, err := os.Stat(filepath.Join(env.Home, ".local", "share", "kidtimer", "kid-bar.json")); err != nil {
		t.Fatal("kid-bar tokens")
	}
	role, err := household.LoadRole(filepath.Join(env.Home, ".local", "share", "kidtimer"))
	if err != nil || role != reverse.RoleKid {
		t.Fatalf("kid role %v %v", role, err)
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
		if str(w["id"]) == pluginID {
			kid = w
		}
	}
	if kid == nil {
		t.Fatalf("kid widget: %s", raw)
	}
	if str(kid["readToken"]) != "" || str(kid["askToken"]) != "" {
		t.Fatal("tokens must not live in shell.json")
	}
	bar, err := readJSON(filepath.Join(env.Home, ".local", "share", "kidtimer", "kid-bar.json"))
	if err != nil || str(bar["readToken"]) == "" || str(bar["askToken"]) == "" {
		t.Fatalf("kid-bar: %v %v", bar, err)
	}
	if str(kid["kidBin"]) != "" {
		t.Fatalf("kidBin must not be written %v", kid["kidBin"])
	}
	if str(bar["url"]) != "http://127.0.0.1:8742" {
		t.Fatalf("url %v", bar["url"])
	}
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "daemon/cmd/kidtimer/setup.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "chownToHomeOwner") {
		t.Fatal("kid setup must chown home files after sudo")
	}
	cfg, err := os.ReadFile(filepath.Join(root, "etc", "kidtimer", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfg), "advertise = true") {
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
	custom := strings.Replace(string(raw), `listen = "127.0.0.1:8742"`, `listen = "0.0.0.0:8742"`, 1)
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
	if !strings.Contains(string(got), `listen = "127.0.0.1:8742"`) || strings.Contains(string(got), "0.0.0.0") {
		t.Fatalf("LAN listen must become loopback: %s", got)
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
	if !strings.Contains(string(got), `listen = "127.0.0.1:8742"`) {
		t.Fatalf("loopback listen: %s", got)
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
	_ = os.RemoveAll(filepath.Join(env.Home, ".config", "omarchy", "plugins", pluginID))
	_ = os.RemoveAll(filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer"))
	_ = os.RemoveAll(filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer.kid"))
	if err := os.Remove(filepath.Join(root, "etc", "systemd", "system", "kidtimer.service")); err != nil {
		t.Fatal(err)
	}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	if _, err := config.ParseFile(cfgPath); err != nil {
		t.Fatalf("repaired config: %v", err)
	}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	doc, err := readJSON(shell)
	if err != nil {
		t.Fatal(err)
	}
	var kid map[string]any
	for _, w := range widgets(doc) {
		if str(w["id"]) == pluginID {
			kid = w
		}
	}
	if kid == nil {
		t.Fatalf("repaired widget: %v", doc)
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".local", "share", "kidtimer", "kid-bar.json")); err != nil {
		t.Fatal("repaired kid-bar")
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".config", "omarchy", "plugins", pluginID, "manifest.json")); err != nil {
		if _, lerr := os.Lstat(filepath.Join(env.Home, ".config", "omarchy", "plugins", pluginID)); lerr != nil {
			t.Fatal("repaired plugin")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "etc", "systemd", "system", "kidtimer.service")); err != nil {
		t.Fatal("repaired unit")
	}
}

func TestSetupCorruptShellFailsClosed(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	if err := setupParent(env); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	if err := os.WriteFile(shell, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setupParent(env); err == nil {
		t.Fatal("corrupt shell.json must fail closed")
	}
	got, err := os.ReadFile(shell)
	if err != nil || string(got) != "{" {
		t.Fatalf("must not replace the bar: %s %v", got, err)
	}
}

func TestSetupKidRepairsStaleTokens(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	if err := writeKidBar(filepath.Join(env.Home, ".local", "share", "kidtimer"), "dead", "dead"); err != nil {
		t.Fatal(err)
	}
	if err := setupKid(env); err != nil {
		t.Fatal(err)
	}
	doc, err := readJSON(filepath.Join(env.Home, ".local", "share", "kidtimer", "kid-bar.json"))
	if err != nil {
		t.Fatal(err)
	}
	if str(doc["readToken"]) == "dead" || str(doc["askToken"]) == "dead" {
		t.Fatal("stale tokens must be reminted")
	}
	if str(doc["readToken"]) == "" || str(doc["askToken"]) == "" {
		t.Fatal("missing remint")
	}
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
	if strings.Contains(body, `exec sudo "$0"`) {
		t.Fatal("must not sudo the installer script")
	}
	if !strings.Contains(body, "sudo install -o root -g root -m 0755") {
		t.Fatal("kid must sudo install the hashed binary")
	}
	if !strings.Contains(body, "/usr/local/bin/kidtimer") {
		t.Fatal("kid system binary")
	}
	if !strings.Contains(body, `HOME}/.local/share/kidtimer/src`) {
		t.Fatal("parent share dir")
	}
	if !strings.Contains(body, "/usr/local/share/kidtimer") {
		t.Fatal("kid share dir")
	}
	if !strings.Contains(body, "manifest.json") {
		t.Fatal("root plugin manifest")
	}
}

func TestBootstrapScriptDoesNotGuessRole(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(repoRoot(t), "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, "omarchy plugin add") {
		t.Fatal("must point at plugin add")
	}
	if strings.Contains(body, "curl") || strings.Contains(body, "| bash") {
		t.Fatal("must not pipe curl to a shell")
	}
	if strings.Contains(body, "releases/download/latest") {
		t.Fatal("must not fetch latest")
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
	script := filepath.Join(repoRoot(t), "install.sh")
	out, err := exec.Command("bash", script, "parent").CombinedOutput()
	if err == nil {
		t.Fatal("expected plugin-add pointer")
	}
	if !strings.Contains(string(out), "omarchy plugin add") {
		t.Fatalf("usage: %s", out)
	}
}

func TestSetupParentMigratesBarePluginID(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	old := filepath.Join(env.Home, ".config", "omarchy", "plugins", legacyPluginID)
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "manifest.json"), []byte(`{"id":"kidtimer"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	if err := ensureWidget(shell, legacyPluginID, map[string]any{"url": "http://127.0.0.1:8742"}); err != nil {
		t.Fatal(err)
	}
	if err := setupParent(env); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(old); !os.IsNotExist(err) {
		t.Fatalf("old plugin leftover %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".config", "omarchy", "plugins", pluginID, "manifest.json")); err != nil {
		if _, lerr := os.Lstat(filepath.Join(env.Home, ".config", "omarchy", "plugins", pluginID)); lerr != nil {
			t.Fatal("namespaced plugin missing")
		}
	}
	doc, err := readJSON(shell)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	n := 0
	for _, w := range widgets(doc) {
		switch str(w["id"]) {
		case pluginID:
			got = w
			n++
		case legacyPluginID, "kidtimer.kid", "kidtimer.parent", "allowance.parent":
			t.Fatalf("legacy widget remains %v", w)
		}
	}
	if n != 1 || got == nil {
		t.Fatalf("want one namespaced widget: %v", doc)
	}
	if str(got["url"]) != "http://127.0.0.1:8742" {
		t.Fatalf("url not preserved %v", got["url"])
	}
	if _, err := os.Stat(filepath.Join(env.Home, ".local", "share", "kidtimer")); err != nil {
		t.Fatal("share dir moved")
	}
}

func TestSetupParentRewritesCenterChip(t *testing.T) {
	root := t.TempDir()
	env := setupEnv{Root: root, Repo: repoRoot(t), Home: filepath.Join(root, "home"), SkipSystemd: true}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	doc := map[string]any{
		"version": 1.0,
		"bar": map[string]any{
			"layout": map[string]any{
				"center": []any{map[string]any{"id": legacyPluginID, "url": "http://127.0.0.1:8742"}},
				"right":  []any{},
			},
		},
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(shell), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shell, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := setupParent(env); err != nil {
		t.Fatal(err)
	}
	got, err := readJSON(shell)
	if err != nil {
		t.Fatal(err)
	}
	bar, _ := got["bar"].(map[string]any)
	layout, _ := bar["layout"].(map[string]any)
	center := asSlice(layout["center"])
	if len(center) != 1 || str(center[0].(map[string]any)["id"]) != pluginID {
		t.Fatalf("center %v", layout["center"])
	}
	if str(center[0].(map[string]any)["url"]) != "http://127.0.0.1:8742" {
		t.Fatalf("center url %v", center[0])
	}
	for _, w := range asSlice(layout["right"]) {
		if str(w.(map[string]any)["id"]) == pluginID {
			t.Fatal("duplicated chip on the right")
		}
	}
}

func TestMigrateKeepsOldPluginUntilNamespacedExists(t *testing.T) {
	home := t.TempDir()
	old := filepath.Join(home, ".config", "omarchy", "plugins", legacyPluginID)
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "manifest.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(home, ".config", "omarchy", "shell.json")
	if err := ensureWidget(shell, legacyPluginID, nil); err != nil {
		t.Fatal(err)
	}
	if err := migrateInstalledPlugin(home); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(old, "manifest.json")); err != nil {
		t.Fatal("old plugin must stay until the namespaced plugin is installed")
	}
	ids := widgetIDs(t, shell)
	if !strings.Contains(ids, `"kidtimer"`) {
		t.Fatalf("bar still names the old chip until migrate can run: %s", ids)
	}
}

func TestRetireOldPluginsMaterializesSymlink(t *testing.T) {
	home := t.TempDir()
	old := pluginDir(home, legacyPluginID)
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "manifest.json"), []byte(`{"id":"kidtimer"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := pluginDest(home)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(old, dest); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(home, ".config", "omarchy", "shell.json")
	if err := ensureWidget(shell, legacyPluginID, nil); err != nil {
		t.Fatal(err)
	}
	if err := rewriteWidgetID(shell, legacyPluginID, pluginID); err != nil {
		t.Fatal(err)
	}
	if err := retireOldPlugins(home, shell, dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(old); !os.IsNotExist(err) {
		t.Fatalf("old plugin leftover %v", err)
	}
	st, err := os.Lstat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		t.Fatal("dest still a symlink to the retired folder")
	}
	if _, err := os.Stat(filepath.Join(dest, "manifest.json")); err != nil {
		t.Fatal("materialized manifest")
	}
}

func TestUserHomeFromShare(t *testing.T) {
	if got := userHomeFromShare("/home/ada/.local/share/kidtimer"); got != "/home/ada" {
		t.Fatalf("got %q", got)
	}
	if got := userHomeFromShare("/tmp/kidtimer"); got != "" {
		t.Fatalf("temp share %q", got)
	}
}

func TestMigrateLivePluginRemovesSecondPlugin(t *testing.T) {
	home := t.TempDir()
	share := filepath.Join(home, ".local", "share", "kidtimer")
	if err := os.MkdirAll(share, 0o700); err != nil {
		t.Fatal(err)
	}
	old := pluginDir(home, legacyPluginID)
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "manifest.json"), []byte(`{"id":"kidtimer"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := pluginDest(home)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "manifest.json"), []byte(`{"id":"`+pluginID+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(home, ".config", "omarchy", "shell.json")
	if err := ensureWidget(shell, legacyPluginID, map[string]any{"url": "http://127.0.0.1:8742"}); err != nil {
		t.Fatal(err)
	}
	migrateLivePlugin(share)
	if _, err := os.Lstat(old); !os.IsNotExist(err) {
		t.Fatalf("old plugin leftover %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "manifest.json")); err != nil {
		t.Fatal("namespaced plugin missing")
	}
	doc, err := readJSON(shell)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	var got map[string]any
	for _, w := range widgets(doc) {
		switch str(w["id"]) {
		case pluginID:
			got = w
			n++
		case legacyPluginID, "kidtimer.kid", "kidtimer.parent", "allowance.parent":
			t.Fatalf("second plugin remains %v", w)
		}
	}
	if n != 1 || str(got["url"]) != "http://127.0.0.1:8742" {
		t.Fatalf("migrated widget %v n=%d", got, n)
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
