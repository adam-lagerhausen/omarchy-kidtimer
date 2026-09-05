package enforcer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionEnvFromRuntimeDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "hypr", "sig-abc"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldSock := filepath.Join(dir, "wayland-0")
	newSock := filepath.Join(dir, "wayland-1")
	lock := filepath.Join(dir, "wayland-1.lock")
	for _, p := range []string{oldSock, newSock, lock} {
		if err := os.WriteFile(p, []byte{}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(oldSock, old, old); err != nil {
		t.Fatal(err)
	}
	acct := Account{Username: "sam", Home: "/home/sam", Gid: 1000, Groups: []uint32{1000, 10}}
	env, cred := SessionEnv(1000, dir, "/opt/omarchy", acct, []string{
		"PATH=/usr/bin",
		"HOME=/wrong",
		"WAYLAND_DISPLAY=wayland-9",
	})
	if cred.Uid != 1000 || cred.Gid != 1000 {
		t.Fatalf("cred uid=%d gid=%d", cred.Uid, cred.Gid)
	}
	if len(cred.Groups) != 2 || cred.Groups[1] != 10 {
		t.Fatalf("groups: %v", cred.Groups)
	}
	got := envMap(env)
	if got["OMARCHY_PATH"] != "/opt/omarchy" {
		t.Fatalf("OMARCHY_PATH=%q", got["OMARCHY_PATH"])
	}
	if got["WAYLAND_DISPLAY"] != "wayland-1" {
		t.Fatalf("WAYLAND_DISPLAY=%q", got["WAYLAND_DISPLAY"])
	}
	if got["HYPRLAND_INSTANCE_SIGNATURE"] != "sig-abc" {
		t.Fatalf("sig=%q", got["HYPRLAND_INSTANCE_SIGNATURE"])
	}
	if got["HOME"] != "/home/sam" || got["USER"] != "sam" || got["LOGNAME"] != "sam" {
		t.Fatalf("account: %+v", got)
	}
	if got["XDG_RUNTIME_DIR"] != dir {
		t.Fatalf("runtime=%q", got["XDG_RUNTIME_DIR"])
	}
	if got["DBUS_SESSION_BUS_ADDRESS"] != "unix:path="+filepath.Join(dir, "bus") {
		t.Fatalf("dbus=%q", got["DBUS_SESSION_BUS_ADDRESS"])
	}
	if got["PATH"] != "/usr/bin" {
		t.Fatal("lost PATH")
	}
}

func TestSessionEnvDefaultsOmarchyPath(t *testing.T) {
	env, _ := SessionEnv(1000, t.TempDir(), "", Account{}, nil)
	if envVal(env, "OMARCHY_PATH") != defaultOmarchyPath {
		t.Fatalf("OMARCHY_PATH=%q", envVal(env, "OMARCHY_PATH"))
	}
}

func TestCredentialForUsesGidAndGroups(t *testing.T) {
	cred := CredentialFor(1001, Account{Gid: 1000, Groups: []uint32{1000, 4}})
	if cred.Uid != 1001 {
		t.Fatalf("uid=%d", cred.Uid)
	}
	if cred.Gid == 0 || cred.Gid != 1000 {
		t.Fatalf("gid=%d", cred.Gid)
	}
	if len(cred.Groups) != 2 || cred.Groups[1] != 4 {
		t.Fatalf("groups=%v", cred.Groups)
	}
}

func TestOmarchyLockerRejectsRoot(t *testing.T) {
	if err := (OmarchyLocker{UID: 0}).Lock(); err == nil {
		t.Fatal("root must not lock")
	}
}

func TestLockSysProcAttrOnlyWhenRoot(t *testing.T) {
	cred := CredentialFor(1000, Account{Gid: 1000, Groups: []uint32{1000}})
	if lockSysProcAttr(1000, cred) != nil {
		t.Fatal("session-user daemon must not setuid")
	}
	attr := lockSysProcAttr(0, cred)
	if attr == nil || attr.Credential == nil || attr.Credential.Uid != 1000 {
		t.Fatalf("root must drop to kid: %+v", attr)
	}
}

func TestHyprlandRootUIDSkips(t *testing.T) {
	w, ok, err := (Hyprland{UID: 0}).Active()
	if err != nil || ok || w.PID != 0 {
		t.Fatalf("root hypr: %+v ok=%v err=%v", w, ok, err)
	}
}

func envMap(env []string) map[string]string {
	out := map[string]string{}
	for _, kv := range env {
		k, v, ok := splitEnv(kv)
		if ok {
			out[k] = v
		}
	}
	return out
}

func envVal(env []string, key string) string {
	return envMap(env)[key]
}

func splitEnv(kv string) (string, string, bool) {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return "", "", false
}
