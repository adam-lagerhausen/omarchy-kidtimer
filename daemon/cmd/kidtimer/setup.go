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
	"kidtimer/daemon/internal/reverse"
)

type setupEnv struct {
	Root        string
	Repo        string
	Home        string
	SkipSystemd bool
}

func runSetup(args []string) error {
	if len(args) < 1 || (args[0] != "parent" && args[0] != "kid" && args[0] != "stop-user-bank") {
		return fmt.Errorf("usage: kidtimer setup parent|kid|stop-user-bank")
	}
	role := args[0]
	env, err := parseSetupEnv(args[1:])
	if err != nil {
		return err
	}
	if role == "stop-user-bank" {
		if env.Home == "" {
			return fmt.Errorf("home is required")
		}
		stopUserPickupDaemons(env.Home)
		return nil
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
	return "", fmt.Errorf("could not find omarchy-kidtimer repo (manifest.json + packaging)")
}

func isRepo(dir string) bool {
	_, err1 := os.Stat(filepath.Join(dir, "manifest.json"))
	_, err2 := os.Stat(filepath.Join(dir, "packaging", "config.kid.toml"))
	_, err3 := os.Stat(filepath.Join(dir, "BarWidget.qml"))
	return err1 == nil && err2 == nil && err3 == nil
}

const (
	pluginID       = "io.github.adam-lagerhausen.kidtimer"
	legacyPluginID = "kidtimer"
)

var retiredPluginIDs = []string{
	legacyPluginID,
	"kidtimer.parent",
	"kidtimer.kid",
	"allowance.parent",
}

var barSections = []string{"left", "center", "right"}

func pluginDest(home string) string {
	return pluginDir(home, pluginID)
}

func pluginDir(home, id string) string {
	return filepath.Join(home, ".config", "omarchy", "plugins", id)
}

func userHomeFromShare(share string) string {
	share = filepath.ToSlash(filepath.Clean(share))
	const suffix = "/.local/share/kidtimer"
	if !strings.HasSuffix(share, suffix) {
		return ""
	}
	return filepath.FromSlash(strings.TrimSuffix(share, suffix))
}

func migrateLivePlugin(share string) {
	home := userHomeFromShare(share)
	if home == "" {
		return
	}
	if err := migrateInstalledPlugin(home); err != nil {
		fmt.Fprintf(os.Stderr, "kidtimer: plugin id migrate: %v\n", err)
	}
}

func migrateInstalledPlugin(home string) error {
	dest := pluginDest(home)
	if _, err := os.Stat(filepath.Join(dest, "manifest.json")); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	shell := filepath.Join(home, ".config", "omarchy", "shell.json")
	if err := rewriteWidgetID(shell, legacyPluginID, pluginID); err != nil {
		return err
	}
	return retireOldPlugins(home, shell, dest)
}

func retireOldPlugins(home, shell, dest string) error {
	if dest != "" {
		if err := materializeIfLinkToRetired(home, dest); err != nil {
			return err
		}
	}
	destAbs := ""
	if dest != "" {
		destAbs, _ = filepath.Abs(dest)
	}
	for _, id := range retiredPluginIDs {
		dir := pluginDir(home, id)
		if destAbs != "" {
			if dirAbs, err := filepath.Abs(dir); err == nil && dirAbs == destAbs {
				continue
			}
		}
		_ = os.RemoveAll(dir)
		if err := removeWidget(shell, id); err != nil {
			return err
		}
	}
	return nil
}

func materializeIfLinkToRetired(home, dest string) error {
	target, err := os.Readlink(dest)
	if err != nil {
		return nil
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(dest), target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	for _, id := range retiredPluginIDs {
		oldAbs, err := filepath.Abs(pluginDir(home, id))
		if err != nil {
			continue
		}
		if oldAbs == target {
			return materializePluginLink(dest)
		}
	}
	return nil
}

func materializePluginLink(dest string) error {
	target, err := os.Readlink(dest)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(dest), target)
	}
	target, err = filepath.EvalSymlinks(target)
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dest), ".kidtimer-plugin-*")
	if err != nil {
		return err
	}
	keep := tmp
	defer func() {
		if keep != "" {
			_ = os.RemoveAll(keep)
		}
	}()
	if err := copyTree(target, tmp); err != nil {
		return err
	}
	if err := os.Remove(dest); err != nil {
		return err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return err
	}
	keep = ""
	return nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, rel)
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, out)
		}
		if info.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		return copyFile(path, out, info.Mode().Perm())
	})
}

func placePlugin(repo, dest string) error {
	if st, err := os.Stat(filepath.Join(dest, "manifest.json")); err == nil && !st.IsDir() {
		return nil
	}
	return linkPlugin(repo, dest)
}

func kidComputerLocked(env setupEnv) error {
	share := filepath.Join(env.Home, ".local", "share", "kidtimer")
	role, err := household.LoadRole(share)
	if err != nil {
		return err
	}
	if role == reverse.RoleKid {
		return fmt.Errorf("this computer is already a kid; uninstall Kidtimer to use it as the parent desk")
	}
	if _, err := os.Stat(filepath.Join(env.etc(), "config.toml")); err == nil {
		return fmt.Errorf("this computer is already a kid; uninstall Kidtimer to use it as the parent desk")
	}
	if _, err := os.Stat(env.unitPath()); err == nil {
		return fmt.Errorf("this computer is already a kid; uninstall Kidtimer to use it as the parent desk")
	}
	return nil
}

func setupParent(env setupEnv) error {
	if env.Home == "" {
		return fmt.Errorf("home is required")
	}
	if err := kidComputerLocked(env); err != nil {
		return err
	}
	bin := filepath.Join(env.Home, ".local", "bin", "kidtimer")
	if err := installBinary(bin); err != nil {
		return err
	}
	if err := placePlugin(env.Repo, pluginDest(env.Home)); err != nil {
		return err
	}
	share := filepath.Join(env.Home, ".local", "share", "kidtimer")
	if err := os.MkdirAll(share, 0o700); err != nil {
		return err
	}
	if err := household.WriteRole(share, reverse.RoleParent); err != nil {
		return err
	}
	if err := household.ImportIfEmpty(share); err != nil {
		return err
	}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	if err := migrateInstalledPlugin(env.Home); err != nil {
		return err
	}
	return ensureWidget(shell, pluginID, nil)
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
	if err := placePlugin(env.Repo, pluginDest(env.Home)); err != nil {
		return err
	}
	share := filepath.Join(env.Home, ".local", "share", "kidtimer")
	if err := os.MkdirAll(share, 0o700); err != nil {
		return err
	}
	if err := household.WriteRole(share, reverse.RoleKid); err != nil {
		return err
	}
	if err := writeKidBar(share, readTok, askTok); err != nil {
		return err
	}
	shell := filepath.Join(env.Home, ".config", "omarchy", "shell.json")
	if err := migrateInstalledPlugin(env.Home); err != nil {
		return err
	}
	if err := ensureWidget(shell, pluginID, nil); err != nil {
		return err
	}
	if err := copyFile(filepath.Join(env.Repo, "packaging", "kidtimer.service"), env.unitPath(), 0o644); err != nil {
		return err
	}
	if err := chownToHomeOwner(env.Home, share, pluginDest(env.Home), shell); err != nil {
		return err
	}
	if env.SkipSystemd {
		return nil
	}
	return startKidUnit()
}

func chownToHomeOwner(home string, paths ...string) error {
	if home == "" {
		return nil
	}
	fi, err := os.Stat(home)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	uid, gid := int(st.Uid), int(st.Gid)
	if os.Getuid() == uid {
		return nil
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		err := filepath.Walk(p, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			return os.Chown(path, uid, gid)
		})
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func startKidUnit() error {
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
	return pinKidLoopback(dst, cfg)
}

func pinKidLoopback(dst string, cfg *config.Config) error {
	needAdv := cfg.Advertise
	needListen := cfg.Listen == "" || strings.Contains(cfg.Listen, "0.0.0.0")
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
			out = advertiseLine.ReplaceAllString(out, "advertise = false")
		} else {
			out = "advertise = false\n" + out
		}
	}
	if needListen {
		if listenLine.MatchString(out) {
			out = listenLine.ReplaceAllString(out, `listen = "127.0.0.1:8742"`)
		} else {
			out = "listen = \"127.0.0.1:8742\"\n" + out
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
	if r, a, ok := existingKidTokens(home); ok {
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

func writeKidBar(share, readTok, askTok string) error {
	doc := map[string]any{
		"url":       "http://127.0.0.1:8742",
		"readToken": readTok,
		"askToken":  askTok,
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(share, "kid-bar.json"), append(raw, '\n'), 0o600)
}

func existingKidTokens(home string) (readTok, askTok string, ok bool) {
	share := filepath.Join(home, ".local", "share", "kidtimer", "kid-bar.json")
	if doc, err := readJSON(share); err == nil {
		readTok = str(doc["readToken"])
		askTok = str(doc["askToken"])
		if readTok != "" && askTok != "" {
			return readTok, askTok, true
		}
	}
	doc, err := readJSON(filepath.Join(home, ".config", "omarchy", "shell.json"))
	if err != nil {
		return "", "", false
	}
	for _, w := range widgets(doc) {
		id := str(w["id"])
		if id != "kidtimer.kid" && id != legacyPluginID && id != pluginID {
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

func loadShell(path string) (map[string]any, error) {
	doc, err := readJSON(path)
	if err == nil {
		return doc, nil
	}
	if os.IsNotExist(err) {
		return emptyShell(), nil
	}
	return nil, err
}

func rewriteWidgetID(shell, from, to string) error {
	if from == "" || to == "" || from == to {
		return nil
	}
	doc, err := readJSON(shell)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	bar, _ := doc["bar"].(map[string]any)
	if bar == nil {
		return nil
	}
	layout, _ := bar["layout"].(map[string]any)
	if layout == nil {
		return nil
	}
	hasTo := layoutHasID(layout, to)
	changed := false
	for _, key := range barSections {
		list := asSlice(layout[key])
		if list == nil {
			continue
		}
		out := make([]any, 0, len(list))
		sectionChanged := false
		for _, raw := range list {
			w, ok := raw.(map[string]any)
			if !ok {
				out = append(out, raw)
				continue
			}
			if str(w["id"]) != from {
				out = append(out, raw)
				continue
			}
			sectionChanged = true
			changed = true
			if hasTo {
				continue
			}
			w["id"] = to
			out = append(out, w)
			hasTo = true
		}
		if sectionChanged {
			layout[key] = out
		}
	}
	if !changed {
		return nil
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(shell, append(raw, '\n'), 0o644)
}

func layoutHasID(layout map[string]any, id string) bool {
	for _, w := range widgetsFromLayout(layout) {
		if str(w["id"]) == id {
			return true
		}
	}
	return false
}

func ensureWidget(shell, id string, extra map[string]any) error {
	doc, err := loadShell(shell)
	if err != nil {
		return err
	}
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
	found := false
	for _, key := range barSections {
		list := asSlice(layout[key])
		if list == nil {
			continue
		}
		for i, raw := range list {
			w, _ := raw.(map[string]any)
			if str(w["id"]) != id {
				continue
			}
			for k, v := range extra {
				w[k] = v
			}
			list[i] = w
			layout[key] = list
			found = true
			break
		}
		if found {
			break
		}
	}
	if !found {
		right := asSlice(layout["right"])
		w := map[string]any{"id": id}
		for k, v := range extra {
			w[k] = v
		}
		layout["right"] = append(right, w)
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(shell, append(raw, '\n'), 0o644)
}

func removeWidget(shell, id string) error {
	doc, err := readJSON(shell)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	bar, _ := doc["bar"].(map[string]any)
	if bar == nil {
		return nil
	}
	layout, _ := bar["layout"].(map[string]any)
	if layout == nil {
		return nil
	}
	changed := false
	for _, key := range barSections {
		list := asSlice(layout[key])
		if list == nil {
			continue
		}
		out := make([]any, 0, len(list))
		for _, raw := range list {
			w, _ := raw.(map[string]any)
			if str(w["id"]) == id {
				changed = true
				continue
			}
			out = append(out, raw)
		}
		if len(out) != len(list) {
			layout[key] = out
		}
	}
	if !changed {
		return nil
	}
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
	return widgetsFromLayout(layout)
}

func widgetsFromLayout(layout map[string]any) []map[string]any {
	var out []map[string]any
	for _, key := range barSections {
		for _, raw := range asSlice(layout[key]) {
			if w, ok := raw.(map[string]any); ok {
				out = append(out, w)
			}
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
