package enforcer

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const defaultOmarchyPath = "/usr/share/omarchy"

type Account struct {
	Username string
	Home     string
	Gid      uint32
	Groups   []uint32
}

type OmarchyLocker struct {
	UID         int
	RuntimeRoot string
	OmarchyPath string
	Lookup      func(int) (Account, error)
	Environ     []string
}

func (l OmarchyLocker) Lock() error {
	if l.UID <= 0 {
		return fmt.Errorf("no graphical session")
	}
	acct, err := l.account()
	if err != nil {
		return err
	}
	env, cred := SessionEnv(l.UID, l.runtimeDir(), l.omarchyPath(), acct, l.parentEnv())
	cmd := exec.Command("omarchy", "system", "lock")
	cmd.Env = env
	if attr := lockSysProcAttr(os.Getuid(), cred); attr != nil {
		cmd.SysProcAttr = attr
	}
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func lockSysProcAttr(euid int, cred *syscall.Credential) *syscall.SysProcAttr {
	if euid != 0 || cred == nil {
		return nil
	}
	return &syscall.SysProcAttr{Credential: cred}
}

func (l OmarchyLocker) account() (Account, error) {
	if l.Lookup != nil {
		return l.Lookup(l.UID)
	}
	return LookupAccount(l.UID)
}

func (l OmarchyLocker) runtimeDir() string {
	root := l.RuntimeRoot
	if root == "" {
		root = "/run/user"
	}
	return filepath.Join(root, strconv.Itoa(l.UID))
}

func (l OmarchyLocker) omarchyPath() string {
	if l.OmarchyPath != "" {
		return l.OmarchyPath
	}
	if p := os.Getenv("OMARCHY_PATH"); p != "" {
		return p
	}
	return defaultOmarchyPath
}

func (l OmarchyLocker) parentEnv() []string {
	if l.Environ != nil {
		return l.Environ
	}
	return os.Environ()
}

func LookupAccount(uid int) (Account, error) {
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return Account{}, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return Account{}, err
	}
	acct := Account{Username: u.Username, Home: u.HomeDir, Gid: uint32(gid)}
	gids, err := u.GroupIds()
	if err != nil {
		return acct, nil
	}
	for _, g := range gids {
		n, err := strconv.Atoi(g)
		if err != nil {
			continue
		}
		acct.Groups = append(acct.Groups, uint32(n))
	}
	return acct, nil
}

func SessionEnv(uid int, runtimeDir, omarchyPath string, acct Account, parentEnv []string) ([]string, *syscall.Credential) {
	if omarchyPath == "" {
		omarchyPath = defaultOmarchyPath
	}
	extra := map[string]string{
		"XDG_RUNTIME_DIR":          runtimeDir,
		"OMARCHY_PATH":             omarchyPath,
		"DBUS_SESSION_BUS_ADDRESS": "unix:path=" + filepath.Join(runtimeDir, "bus"),
	}
	if acct.Home != "" {
		extra["HOME"] = acct.Home
	}
	if acct.Username != "" {
		extra["USER"] = acct.Username
		extra["LOGNAME"] = acct.Username
	}
	if display := waylandDisplay(runtimeDir); display != "" {
		extra["WAYLAND_DISPLAY"] = display
	}
	if sig := hyprSignature(runtimeDir); sig != "" {
		extra["HYPRLAND_INSTANCE_SIGNATURE"] = sig
	}
	return overlayEnv(parentEnv, extra), CredentialFor(uid, acct)
}

func CredentialFor(uid int, acct Account) *syscall.Credential {
	return &syscall.Credential{
		Uid:    uint32(uid),
		Gid:    acct.Gid,
		Groups: acct.Groups,
	}
}

func overlayEnv(parent []string, extra map[string]string) []string {
	skip := make(map[string]bool, len(extra))
	for k := range extra {
		skip[k] = true
	}
	out := make([]string, 0, len(parent)+len(extra))
	for _, kv := range parent {
		k, _, ok := strings.Cut(kv, "=")
		if !ok || skip[k] {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}

func waylandDisplay(runtimeDir string) string {
	entries, err := os.ReadDir(runtimeDir)
	if err != nil {
		return ""
	}
	var best string
	var bestTime time.Time
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "wayland-") || strings.HasSuffix(name, ".lock") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestTime) {
			best = name
			bestTime = info.ModTime()
		}
	}
	return best
}
