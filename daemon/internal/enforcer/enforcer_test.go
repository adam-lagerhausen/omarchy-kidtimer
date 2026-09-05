package enforcer

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"kidtimer/daemon/internal/bank"
	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/look"
)

func TestMinecraftFocusSpendsThenPauses(t *testing.T) {
	e, rec, parent := setup(t, false, afternoon)
	e.Focus = &StaticFocus{W: Window{Class: "kidtimer-lab", PID: 4242}, OK: true}

	before, _ := e.Bank.Remaining("fun")
	for i := 0; i < 10; i++ {
		if err := e.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	after, _ := e.Bank.Remaining("fun")
	if after != before-10 {
		t.Fatalf("spent: %d -> %d", before, after)
	}
	if len(rec.Stops) != 0 {
		t.Fatalf("paused early: %v", rec.Stops)
	}

	left, _ := e.Bank.Remaining("fun")
	for i := 0; i < left; i++ {
		if err := e.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(rec.Stops) != 1 || rec.Stops[0] != 4242 {
		t.Fatalf("pause: %v", rec.Stops)
	}
	paused, err := e.Bank.Paused()
	if err != nil {
		t.Fatal(err)
	}
	if len(paused) != 1 || paused[0].Group != "fun" {
		t.Fatalf("recorded: %+v", paused)
	}

	if err := e.Resume("other"); err != nil {
		t.Fatal(err)
	}
	if len(rec.Continues) != 0 {
		t.Fatalf("other must not resume fun: %v", rec.Continues)
	}
	if _, err := e.Bank.Grant(parent, "fun", 30, "own", "resume-fun"); err != nil {
		t.Fatal(err)
	}
	if err := e.Resume("fun"); err != nil {
		t.Fatal(err)
	}
	if len(rec.Continues) != 1 || rec.Continues[0] != 4242 {
		t.Fatalf("cont: %v", rec.Continues)
	}
}

func TestWrongGroupDoesNotResumeFun(t *testing.T) {
	e, rec, _ := setup(t, false, afternoon)
	e.Focus = &StaticFocus{W: Window{Class: "kidtimer-lab", PID: 77}, OK: true}
	spendN(t, e, "fun", remaining(t, e, "fun"))
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(rec.Stops) != 1 || rec.Stops[0] != 77 {
		t.Fatalf("fun pause: %v", rec.Stops)
	}
	if err := e.Resume("other"); err != nil {
		t.Fatal(err)
	}
	if len(rec.Continues) != 0 {
		t.Fatalf("other must not resume fun: %v", rec.Continues)
	}
}

func TestSleepChildStopAndCont(t *testing.T) {
	cmd := exec.Command("sleep", "3600")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	pid := cmd.Process.Pid
	time.Sleep(50 * time.Millisecond)

	e, rec, parent := setup(t, false, afternoon)
	e.Signals = &Recorder{
		Rec:      rec,
		Inner:    UnixSignaler{},
		AllowPID: map[int]bool{pid: true},
	}
	e.Focus = &StaticFocus{W: Window{Class: "kidtimer-lab", PID: pid}, OK: true}

	left, _ := e.Bank.Remaining("fun")
	for i := 0; i < left; i++ {
		if err := e.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if got := procState(t, pid); got != 'T' {
		t.Fatalf("after SIGSTOP state=%c", got)
	}

	if _, err := e.Bank.Grant(parent, "fun", 60, "resume", "fun"); err != nil {
		t.Fatal(err)
	}
	if err := e.Resume("fun"); err != nil {
		t.Fatal(err)
	}
	if got := procState(t, pid); got == 'T' {
		t.Fatal("still stopped after SIGCONT")
	}
	if len(rec.Continues) != 1 || rec.Continues[0] != pid {
		t.Fatalf("cont: %v", rec.Continues)
	}
}

func TestAlwaysOnDoesNotSpend(t *testing.T) {
	e, rec, _ := setup(t, false, afternoon)
	before := remaining(t, e, "fun")
	e.Focus = &StaticFocus{W: Window{Class: "", PID: 1}, OK: true}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	e.Focus = &StaticFocus{W: Window{Class: "omarchy-shell", PID: 2}, OK: true}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != before {
		t.Fatal("always-on spent fun")
	}
	e.Focus = &StaticFocus{W: Window{Class: "Alacritty", PID: 9}, OK: true}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != before-1 {
		t.Fatal("chrome-class windows must spend the clock")
	}
	if len(rec.Stops) != 0 {
		t.Fatalf("signaled: %v", rec.Stops)
	}
}

func TestHyprlandDownSkips(t *testing.T) {
	e, rec, _ := setup(t, false, afternoon)
	before := remaining(t, e, "fun")
	e.Focus = &StaticFocus{Err: os.ErrNotExist}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	e.Focus = &StaticFocus{OK: false}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != before {
		t.Fatal("spent while hyprland down")
	}
	if rec.Locks != 0 {
		t.Fatal("locked while hyprland down")
	}
}

func TestIdleAndLockedDoNotSpend(t *testing.T) {
	e, rec, _ := setup(t, false, afternoon)
	e.Focus = &StaticFocus{W: Window{Class: "PrismLauncher", PID: 7}, OK: true}
	e.Session = StaticSession{IsIdle: true}
	before := remaining(t, e, "fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	e.Session = StaticSession{IsLocked: true}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != before {
		t.Fatal("spent while idle/locked")
	}
	if len(rec.Stops) != 0 {
		t.Fatal("paused while idle")
	}
}

func TestBedtimeLockOffNeverLocks(t *testing.T) {
	e, rec, _ := setup(t, false, bedtime)
	e.Focus = &StaticFocus{W: Window{Class: "PrismLauncher", PID: 8}, OK: true}
	if e.Bank.Config().BedtimeLock {
		t.Fatal("fixture bedtime_lock must stay off")
	}
	if !e.Bank.BedtimeActive() {
		t.Fatal("clock should be inside bedtime")
	}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if rec.Locks != 0 {
		t.Fatal("bedtime_lock off must not lock")
	}
}

func TestBedtimeLockOnDoesNotSpend(t *testing.T) {
	e, rec, parent := setup(t, false, bedtime)
	e.Bank.Config().BedtimeLock = true
	if err := e.Bank.SetParentPIN(parent, "1234", ""); err != nil {
		t.Fatal(err)
	}
	e.Focus = &StaticFocus{W: Window{Class: "PrismLauncher", PID: 8}, OK: true}
	before := remaining(t, e, "fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if rec.Locks != 0 {
		t.Fatalf("must not session-lock: %d", rec.Locks)
	}
	if remaining(t, e, "fun") != before {
		t.Fatal("spent during bedtime overlay")
	}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != before {
		t.Fatal("bedtime is a window, not a pulse")
	}
}

func TestBedtimeHoldSkipsOverlayFreeze(t *testing.T) {
	e, rec, parent := setup(t, false, bedtime)
	e.Bank.Config().BedtimeLock = true
	if err := e.Bank.SetParentPIN(parent, "1234", ""); err != nil {
		t.Fatal(err)
	}
	_, askTok, err := e.Bank.Mint(parent, bank.MintSpec{Name: "kid-bar", Kind: bank.KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Bank.PinGrant(askTok, "1234", 600); err != nil {
		t.Fatal(err)
	}
	e.Focus = &StaticFocus{W: Window{Class: "kidtimer-lab", PID: 8}, OK: true}
	before := remaining(t, e, "fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if rec.Locks != 0 {
		t.Fatal("hold must not session-lock")
	}
	if remaining(t, e, "fun") != before-1 {
		t.Fatal("hold: time still counts")
	}
}

func TestBedtimeWithoutPinStillSpends(t *testing.T) {
	e, rec, _ := setup(t, false, bedtime)
	e.Bank.Config().BedtimeLock = true
	e.Focus = &StaticFocus{W: Window{Class: "PrismLauncher", PID: 8}, OK: true}
	before := remaining(t, e, "fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if rec.Locks != 0 {
		t.Fatal("no pin must not lock")
	}
	if remaining(t, e, "fun") != before-1 {
		t.Fatal("no pin: time still counts")
	}
}

func TestParentLockRemoteOnFreezes(t *testing.T) {
	e, rec, parent := setup(t, false, afternoon)
	e.Bank.Config().RemoteLock = true
	if err := e.Bank.SetParentPIN(parent, "1234", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.Bank.SetParentLock(parent, true); err != nil {
		t.Fatal(err)
	}
	before := remaining(t, e, "fun")
	e.Focus = &StaticFocus{W: Window{Class: "kidtimer-lab", PID: 8}, OK: true}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if rec.Locks != 0 {
		t.Fatalf("must not session-lock: %d", rec.Locks)
	}
	if remaining(t, e, "fun") != before {
		t.Fatal("spent while parent-locked overlay")
	}
}

func TestParentLockWithoutUIDStillFreezes(t *testing.T) {
	e, rec, parent := setup(t, false, afternoon)
	e.Bank.Config().RemoteLock = true
	if err := e.Bank.SetParentPIN(parent, "1234", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.Bank.SetParentLock(parent, true); err != nil {
		t.Fatal(err)
	}
	e.SessionUID = func() int { return 0 }
	before := remaining(t, e, "fun")
	e.Focus = &StaticFocus{W: Window{Class: "kidtimer-lab", PID: 8}, OK: true}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if rec.Locks != 0 {
		t.Fatalf("session lock: %d", rec.Locks)
	}
	if remaining(t, e, "fun") != before {
		t.Fatal("overlay freeze does not need a session uid")
	}
}

func TestParentLockSkipsWhenAlreadyLocked(t *testing.T) {
	e, rec, parent := setup(t, false, afternoon)
	e.Bank.Config().RemoteLock = true
	if err := e.Bank.SetParentPIN(parent, "1234", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.Bank.SetParentLock(parent, true); err != nil {
		t.Fatal(err)
	}
	sess := &MutableSession{LockedHint: true}
	e.Session = sess
	before := remaining(t, e, "fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if rec.Locks != 0 {
		t.Fatalf("session lock: %d", rec.Locks)
	}
	if remaining(t, e, "fun") != before {
		t.Fatal("spent while overlay freeze")
	}
}

func TestParentLockRemoteOffDoesNotLock(t *testing.T) {
	e, rec, parent := setup(t, false, afternoon)
	if e.Bank.Config().RemoteLock {
		t.Fatal("parent-lab remote_lock must stay off")
	}
	if err := e.Bank.SetParentLock(parent, true); err != nil {
		t.Fatal(err)
	}
	e.Focus = &StaticFocus{W: Window{Class: "kidtimer-lab", PID: 8}, OK: true}
	mc, _ := e.Bank.Remaining("fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if rec.Locks != 0 {
		t.Fatal("remote_lock off must not lock for parent lock")
	}
	if remaining(t, e, "fun") == mc {
		t.Fatal("parent-lab should still spend while parent-locked with remote_lock off")
	}
}

func TestLabModeIgnoresNonLabClass(t *testing.T) {
	e, rec, _ := setup(t, true, afternoon)
	e.Focus = &StaticFocus{W: Window{Class: "PrismLauncher", PID: 11}, OK: true}
	before := remaining(t, e, "fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != before {
		t.Fatal("lab mode spent a non-lab window")
	}
	if len(rec.Stops) != 0 {
		t.Fatal("lab mode paused prism")
	}

	e.Focus = &StaticFocus{W: Window{Class: "kidtimer-lab", PID: 12}, OK: true}
	before = remaining(t, e, "fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != before-1 {
		t.Fatalf("lab fun remaining: %d", remaining(t, e, "fun"))
	}
}

func TestAnyWindowSpendsAndGrantKeepsSpent(t *testing.T) {
	e, rec, parent := setup(t, false, afternoon)
	e.Focus = &StaticFocus{W: Window{Class: "google-chrome", PID: 3}, OK: true}
	funBefore := remaining(t, e, "fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != funBefore-1 {
		t.Fatal("chrome must spend the clock")
	}
	if len(rec.Stops) != 0 {
		t.Fatalf("paused: %v", rec.Stops)
	}
	st, err := e.Bank.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if st.Spent["fun"] != 1 {
		t.Fatalf("spent: %+v", st.Spent)
	}
	if len(st.Today) != 1 || st.Today[0].Start != 15*60 || st.Today[0].Dur != 1 {
		t.Fatalf("today: %+v", st.Today)
	}
	if _, err := e.Bank.Grant(parent, "fun", 600, "+10", "plus10"); err != nil {
		t.Fatal(err)
	}
	st, err = e.Bank.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if st.Spent["fun"] != 1 {
		t.Fatalf("grant changed spent: %+v", st.Spent)
	}
}

func TestEmptyLockFreezesWhenRemainingZero(t *testing.T) {
	cfg, err := config.ParseFile(packagingPath(t, "config.kid.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.EmptyLock || cfg.Lab.Enabled {
		t.Fatal("kid empty_lock must be on and lab off")
	}
	b := openBank(t, cfg, afternoon)
	_, parent, err := b.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	putClassicLook(t, b, parent)
	if err := b.SetParentPIN(parent, "1234", ""); err != nil {
		t.Fatal(err)
	}
	rec := &Rec{}
	e := &Enforcer{
		Bank:    b,
		Focus:   &StaticFocus{W: Window{Class: "google-chrome", PID: 3}, OK: true},
		Session: StaticSession{},
		Signals: &Recorder{Rec: rec},
		Locker:  &CountingLocker{Rec: rec},
	}
	spendN(t, e, "fun", remaining(t, e, "fun"))
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if rec.Locks != 0 {
		t.Fatalf("must not session-lock: %d", rec.Locks)
	}
	if len(rec.Stops) != 0 {
		t.Fatalf("kid empty must overlay, not pause: %v", rec.Stops)
	}
}

func TestEmptyLockOffDoesNotLock(t *testing.T) {
	e, rec, _ := setup(t, false, afternoon)
	if e.Bank.Config().EmptyLock {
		t.Fatal("parent-lab empty_lock must stay off")
	}
	spendN(t, e, "fun", remaining(t, e, "fun"))
	e.Focus = &StaticFocus{W: Window{Class: "kidtimer-lab", PID: 5}, OK: true}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if rec.Locks != 0 {
		t.Fatal("empty_lock off must not session-lock")
	}
	if len(rec.Stops) != 1 || rec.Stops[0] != 5 {
		t.Fatalf("lab empty should still pause: %v", rec.Stops)
	}
}

func TestNeverSignalSkips(t *testing.T) {
	e, rec, _ := setup(t, false, afternoon)
	e.Signals.(*Recorder).Names = map[int]string{99: "sshd"}
	left, _ := e.Bank.Remaining("fun")
	e.Focus = &StaticFocus{W: Window{Class: "kidtimer-lab", PID: 99}, OK: true}
	for i := 0; i < left+1; i++ {
		if err := e.Tick(); err != nil {
			t.Fatal(err)
		}
	}
	if len(rec.Stops) != 0 {
		t.Fatalf("signaled sshd: %v", rec.Stops)
	}
}

func TestEnforcerOffRecordsWithoutSignals(t *testing.T) {
	cfg := loadParentLab(t)
	if cfg.Enforcer {
		t.Fatal("parent-lab enforcer must stay off")
	}
	b := openBank(t, cfg, afternoon)
	_, parent, err := b.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	putClassicLook(t, b, parent)
	empty := &Enforcer{Bank: b}
	spendN(t, empty, "fun", remaining(t, empty, "fun"))
	rec := &Rec{}
	e := &Enforcer{
		Bank:    b,
		Focus:   &StaticFocus{W: Window{Class: "kidtimer-lab", PID: 5}, OK: true},
		Session: StaticSession{},
		Signals: &Recorder{Rec: rec},
		Locker:  &CountingLocker{Rec: rec},
	}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if len(rec.Stops) != 0 || rec.Locks != 0 {
		t.Fatalf("enforcer off signaled: stops=%v locks=%d", rec.Stops, rec.Locks)
	}
	paused, _ := b.Paused()
	if len(paused) != 1 {
		t.Fatalf("dry-run should still record pause: %+v", paused)
	}
}

func setup(t *testing.T, lab bool, now func() time.Time) (*Enforcer, *Rec, *bank.Token) {
	t.Helper()
	cfg := loadParentLab(t)
	cfg.Enforcer = true
	cfg.Lab.Enabled = lab
	b := openBank(t, cfg, now)
	_, parent, err := b.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	putClassicLook(t, b, parent)
	rec := &Rec{}
	e := &Enforcer{
		Bank:    b,
		Session: StaticSession{},
		Signals: &Recorder{Rec: rec},
		Locker:  &CountingLocker{Rec: rec},
	}
	return e, rec, parent
}

func putClassicLook(t *testing.T, b *bank.Bank, parent *bank.Token) {
	t.Helper()
	doc, err := b.Look(parent)
	if err != nil {
		t.Fatal(err)
	}
	piles := []look.Pile{{ID: "fun", Name: "Fun"}}
	doc.Piles = piles
	doc.Things = []look.Thing{}
	doc.Apps = map[string]string{}
	doc.Matchers = map[string]look.Matcher{}
	if doc.PileHours["fun"] == 0 {
		doc.PileHours = map[string]int{"fun": 3600}
	}
	for i := range doc.Modes {
		if doc.Modes[i].Kind == look.KindFreetime {
			doc.Modes[i].Hours = map[string]int{}
			continue
		}
		src := b.Config().Modes[doc.Modes[i].ID]
		doc.Modes[i].Hours = map[string]int{"fun": src["fun"]}
	}
	if err := b.PutLook(parent, doc); err != nil {
		t.Fatal(err)
	}
}

func openBank(t *testing.T, cfg *config.Config, now func() time.Time) *bank.Bank {
	t.Helper()
	b, err := bank.Open(filepath.Join(t.TempDir(), "ledger.sqlite"), cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}

func loadParentLab(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func spendN(t *testing.T, e *Enforcer, group string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		res, err := e.Bank.Spend(group)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != bank.SpendTick {
			t.Fatalf("spend %d of %s: %+v", i, group, res)
		}
	}
}

func remaining(t *testing.T, e *Enforcer, group string) int {
	t.Helper()
	n, err := e.Bank.Remaining(group)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func procState(t *testing.T, pid int) byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	i := strings.LastIndex(s, ")")
	if i < 0 || i+2 >= len(s) {
		t.Fatalf("stat: %s", s)
	}
	return s[i+2]
}

func afternoon() time.Time {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return time.Date(2026, 8, 26, 15, 0, 0, 0, loc)
}

func bedtime() time.Time {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return time.Date(2026, 8, 26, 21, 30, 0, 0, loc)
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
