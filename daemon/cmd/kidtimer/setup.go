package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"kidtimer/daemon/internal/advertise"
	"kidtimer/daemon/internal/bank"
	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/netaddr"
	"kidtimer/daemon/internal/reverse"
)

type setupEnv struct {
	Root        string
	Repo        string
	Home        string
	SkipSystemd bool
}

func runSetup(args []string) error {
	if len(args) < 1 || (args[0] != "parent" && args[0] != "kid") {
		return fmt.Errorf("usage: kidtimer setup parent|kid")
	}
	role := args[0]
	env, err := parseSetupEnv(args[1:])
	if err != nil {
		return err
	}
	if role == "parent" {
		return setupParent(env)
	}
	return setupKid(env)
}

func parseSetupEnv(args []string) (setupEnv, error) {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	root := fs.String("root", "", "test prefix; also skips systemd")
	repo := fs.String("repo", "", "omarchy-kidtimer checkout")
	home := fs.String("home", "", "target user home")
	skip := fs.Bool("skip-systemd", false, "do not enable the unit")
	if err := fs.Parse(args); err != nil {
		return setupEnv{}, err
	}
	env := setupEnv{Root: *root, Repo: *repo, Home: *home, SkipSystemd: *skip || *root != ""}
	if env.Repo == "" {
		found, err := findRepo("")
		if err != nil {
			return setupEnv{}, err
		}
		env.Repo = found
	}
	if env.Home == "" {
		env.Home = defaultHome(env.Root)
	}
	return env, nil
}

func defaultHome(root string) string {
	if root != "" {
		return filepath.Join(root, "home")
	}
	if os.Getuid() == 0 {
		if home := homeFromEnv(); home != "" {
			return home
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

func homeFromEnv() string {
	if sudo := os.Getenv("SUDO_USER"); sudo != "" {
		if u, err := user.Lookup(sudo); err == nil && u.HomeDir != "" {
			return u.HomeDir
		}
	}
	if uid := os.Getenv("PKEXEC_UID"); uid != "" {
		if u, err := user.LookupId(uid); err == nil && u.HomeDir != "" {
			return u.HomeDir
		}
	}
	return ""
}

func findRepo(start string) (string, error) {
	var seeds []string
	if start != "" {
		seeds = append(seeds, start)
	}
	if cwd, err := os.Getwd(); err == nil {
		seeds = append(seeds, cwd)
	}
	if exe, err := os.Executable(); err == nil {
		seeds = append(seeds, filepath.Dir(exe))
	}
	for _, seed := range seeds {
		dir := seed
		for i := 0; i < 8; i++ {
			if isRepo(dir) {
				return dir, nil
			}
			next := filepath.Dir(dir)
			if next == dir {
				break
			}
			dir = next
		}
	}
	return "", fmt.Errorf("could not find omarchy-kidtimer repo (plugin-parent + packaging)")
}

func isRepo(dir string) bool {
	_, err1 := os.Stat(filepath.Join(dir, "plugin-parent", "manifest.json"))
	_, err2 := os.Stat(filepath.Join(dir, "plugin-kid", "manifest.json"))
	_, err3 := os.Stat(filepath.Join(dir, "packaging", "config.kid.toml"))
	return err1 == nil && err2 == nil && err3 == nil
}

func setupParent(env setupEnv) error {
	if env.Home == "" {
		return fmt.Errorf("home is required")
	}
	bin := filepath.Join(env.Home, ".local", "bin", "kidtimer")
	if err := installBinary(bin); err != nil {
		return err
	}
	if err := linkPlugin(filepath.Join(env.Repo, "plugin-parent"), filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer.parent")); err != nil {
		return err
	}
	share := filepath.Join(env.Home, ".local", "share", "kidtimer")
	if err := os.MkdirAll(share, 0o700); err != nil {
		return err
	}
	if err := household.WriteRole(share, reverse.RoleParent); err != nil {
		return err
	}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	if err := ensureWidget(shell, "kidtimer.parent", map[string]any{
		"parentBin": bin,
	}); err != nil {
		return err
	}
	return nil
}

func (env setupEnv) etc() string {
	if env.Root != "" {
		return filepath.Join(env.Root, "etc", "kidtimer")
	}
	return "/etc/kidtimer"
}

func (env setupEnv) lib() string {
	if env.Root != "" {
		return filepath.Join(env.Root, "var", "lib", "kidtimer")
	}
	return "/var/lib/kidtimer"
}

func (env setupEnv) kidBin() string {
	if env.Root != "" {
		return filepath.Join(env.Root, "usr", "local", "bin", "kidtimer")
	}
	return "/usr/local/bin/kidtimer"
}

func (env setupEnv) unitPath() string {
	if env.Root != "" {
		return filepath.Join(env.Root, "etc", "systemd", "system", "kidtimer.service")
	}
	return "/etc/systemd/system/kidtimer.service"
}

func setupKid(env setupEnv) error {
	if env.Home == "" {
		return fmt.Errorf("home is required")
	}
	if err := installBinary(env.kidBin()); err != nil {
		return err
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "kid"
	}
	cfgPath := filepath.Join(env.etc(), "config.toml")
	if err := ensureKidConfig(filepath.Join(env.Repo, "packaging", "config.kid.toml"), cfgPath, host); err != nil {
		return err
	}
	if err := os.MkdirAll(env.lib(), 0o755); err != nil {
		return err
	}
	if _, err := advertise.MachineID(filepath.Join(env.lib(), "machine-id")); err != nil {
		return err
	}
	readTok, askTok, err := mintKidBar(filepath.Join(env.lib(), "ledger.sqlite"), filepath.Join(env.etc(), "config.toml"), env.Home)
	if err != nil {
		return err
	}
	if err := linkPlugin(filepath.Join(env.Repo, "plugin-kid"), filepath.Join(env.Home, ".config", "omarchy", "plugins", "kidtimer.kid")); err != nil {
		return err
	}
	if err := ensureWidget(filepath.Join(env.Home, ".config", "omarchy", "shell.json"), "kidtimer.kid", map[string]any{
		"url":       "http://127.0.0.1:8742",
		"readToken": readTok,
		"askToken":  askTok,
		"kidBin":    env.kidBin(),
	}); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(env.Repo, "packaging", "kidtimer.service"), env.unitPath(), 0o644); err != nil {
		return err
	}
	if env.SkipSystemd {
		return nil
	}
	return startKidUnit(env.Home)
}

func startKidUnit(home string) error {
	stopUserPickupDaemons(home)
	enable := exec.Command("systemctl", "enable", "kidtimer")
	enable.Stdout = os.Stdout
	enable.Stderr = os.Stderr
	if err := enable.Run(); err != nil {
		return err
	}
	restart := exec.Command("systemctl", "restart", "kidtimer")
	restart.Stdout = os.Stdout
	restart.Stderr = os.Stderr
	return restart.Run()
}

func userPickupLedger(home string) string {
	return filepath.Join(home, ".local/share/kidtimer", "ledger.sqlite")
}

func userBankPIDPath(dbPath string) string {
	dir := filepath.Clean(filepath.Dir(dbPath))
	if !strings.HasSuffix(filepath.ToSlash(dir), "/.local/share/kidtimer") {
		return ""
	}
	return filepath.Join(dir, "daemon.pid")
}

func claimUserBankPID(dbPath string) func() {
	path := userBankPIDPath(dbPath)
	if path == "" {
		return func() {}
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "daemon pid file: %v\n", err)
		return func() {}
	}
	return func() { _ = os.Remove(path) }
}

func userDaemonOwnsLedger(cmdline, ledger string) bool {
	if ledger == "" {
		return false
	}
	s := strings.ReplaceAll(cmdline, "\x00", " ")
	if !strings.Contains(s, "daemon") {
		return false
	}
	return strings.Contains(s, ledger)
}

func stopUserPickupDaemons(home string) {
	ledger := userPickupLedger(home)
	if pidPath := userBankPIDPath(ledger); pidPath != "" {
		stopPIDFile(pidPath, ledger)
	}
	stopMatchingUserDaemons(ledger)
}

func stopPIDFile(pidPath, ledger string) {
	raw, err := os.ReadFile(pidPath)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 1 {
		_ = os.Remove(pidPath)
		return
	}
	if !signalUserDaemon(pid, ledger) {
		_ = os.Remove(pidPath)
	}
}

func stopMatchingUserDaemons(ledger string) {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	self := os.Getpid()
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid == self {
			continue
		}
		_ = signalUserDaemon(pid, ledger)
	}
}

func signalUserDaemon(pid int, ledger string) bool {
	if pid == os.Getpid() {
		return false
	}
	raw, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return false
	}
	if !userDaemonOwnsLedger(string(raw), ledger) {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = p.Signal(syscall.SIGTERM)
	return true
}

var (
	kidNameLine   = regexp.MustCompile(`(?m)^kid_name\s*=\s*".*"`)
	advertiseLine = regexp.MustCompile(`(?m)^advertise\s*=\s*.*$`)
	listenLine    = regexp.MustCompile(`(?m)^listen\s*=\s*.*$`)
)

func ensureKidConfig(src, dst, host string) error {
	cfg, err := config.ParseFile(dst)
	if err != nil {
		return writeKidConfig(src, dst, host)
	}
	return enableKidLAN(dst, cfg)
}

func enableKidLAN(dst string, cfg *config.Config) error {
	needAdv := !cfg.Advertise
	needListen := cfg.Listen == "" || cfg.Listen == netaddr.DefaultListen
	if !needAdv && !needListen {
		return nil
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		return err
	}
	out := string(raw)
	if needAdv {
		if advertiseLine.MatchString(out) {
			out = advertiseLine.ReplaceAllString(out, "advertise = true")
		} else {
			out = "advertise = true\n" + out
		}
	}
	if needListen {
		if listenLine.MatchString(out) {
			out = listenLine.ReplaceAllString(out, `listen = "0.0.0.0:8742"`)
		} else {
			out = "listen = \"0.0.0.0:8742\"\n" + out
		}
	}
	return writeFileAtomic(dst, []byte(out), 0o644)
}

func writeKidConfig(src, dst, host string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	out := kidNameLine.ReplaceAllString(string(raw), `kid_name = "`+host+`"`)
	return writeFileAtomic(dst, []byte(out), 0o644)
}

func mintKidBar(dbPath, cfgPath, home string) (readTok, askTok string, err error) {
	cfg, err := config.ParseFile(cfgPath)
	if err != nil {
		return "", "", err
	}
	b, err := bank.Open(dbPath, cfg, time.Now)
	if err != nil {
		return "", "", err
	}
	defer b.Close()
	n, err := b.TokenCount()
	if err != nil {
		return "", "", err
	}
	if n == 0 {
		if _, _, err := b.SeedParent("bootstrap"); err != nil {
			return "", "", err
		}
	}
	if r, a, ok := existingKidTokens(filepath.Join(home, ".config", "omarchy", "shell.json")); ok {
		if _, err := b.LookupSecret(r); err == nil {
			if _, err := b.LookupSecret(a); err == nil {
				return r, a, nil
			}
		}
	}
	actor := &bank.Token{Kind: bank.KindParent, Name: "setup"}
	readTok, _, err = b.ReplaceToken(actor, bank.MintSpec{Name: "kid-bar-read", Kind: bank.KindRead})
	if err != nil {
		return "", "", err
	}
	askTok, _, err = b.ReplaceToken(actor, bank.MintSpec{Name: "kid-bar", Kind: bank.KindAsk})
	if err != nil {
		return "", "", err
	}
	return readTok, askTok, nil
}

func existingKidTokens(shell string) (readTok, askTok string, ok bool) {
	doc, err := readJSON(shell)
	if err != nil {
		return "", "", false
	}
	for _, w := range widgets(doc) {
		if str(w["id"]) != "kidtimer.kid" {
			continue
		}
		readTok = str(w["readToken"])
		askTok = str(w["askToken"])
		return readTok, askTok, readTok != "" && askTok != ""
	}
	return "", "", false
}

func installBinary(dst string) error {
	src, err := os.Executable()
	if err != nil {
		return err
	}
	return copyFile(src, dst, 0o755)
}

func copyFile(src, dst string, mode os.FileMode) error {
	if sameFile(src, dst) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), "kidtimer-*.new")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, dst)
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "kidtimer-*.new")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func sameFile(a, b string) bool {
	sa, err1 := os.Stat(a)
	sb, err2 := os.Stat(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return os.SameFile(sa, sb)
}

func linkPlugin(src, dst string) error {
	abs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	if cur, err := os.Readlink(dst); err == nil && cur == abs {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	_ = os.RemoveAll(dst)
	return os.Symlink(abs, dst)
}

func emptyShell() map[string]any {
	return map[string]any{
		"bar":     map[string]any{"layout": map[string]any{"right": []any{}}},
		"version": 1.0,
	}
}

func loadShell(path string) map[string]any {
	doc, err := readJSON(path)
	if err == nil {
		return doc
	}
	if !os.IsNotExist(err) {
		_ = os.Rename(path, path+".bak")
	}
	return emptyShell()
}

func ensureWidget(shell, id string, extra map[string]any) error {
	doc := loadShell(shell)
	bar, _ := doc["bar"].(map[string]any)
	if bar == nil {
		bar = map[string]any{}
		doc["bar"] = bar
	}
	layout, _ := bar["layout"].(map[string]any)
	if layout == nil {
		layout = map[string]any{}
		bar["layout"] = layout
	}
	right := asSlice(layout["right"])
	found := false
	for i, raw := range right {
		w, _ := raw.(map[string]any)
		if str(w["id"]) != id {
			continue
		}
		for k, v := range extra {
			w[k] = v
		}
		right[i] = w
		found = true
		break
	}
	if !found {
		w := map[string]any{"id": id}
		for k, v := range extra {
			w[k] = v
		}
		right = append(right, w)
	}
	layout["right"] = right
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(shell, append(raw, '\n'), 0o644)
}

func readJSON(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func widgets(doc map[string]any) []map[string]any {
	bar, _ := doc["bar"].(map[string]any)
	layout, _ := bar["layout"].(map[string]any)
	var out []map[string]any
	for _, raw := range asSlice(layout["right"]) {
		if w, ok := raw.(map[string]any); ok {
			out = append(out, w)
		}
	}
	return out
}

func asSlice(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	default:
		return nil
	}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
