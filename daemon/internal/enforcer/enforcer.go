package enforcer

import (
	"fmt"
	"os"
	"syscall"

	"kidtimer/daemon/internal/bank"
)

type Window struct {
	Class string
	Title string
	PID   int
}

type FocusSource interface {
	Active() (Window, bool, error)
}

type Session interface {
	Locked() bool
	Idle() bool
}

type Signaler interface {
	Stop(pid int) error
	Continue(pid int) error
	Name(pid int) (string, error)
}

type Locker interface {
	Lock() error
}

type UnixSignaler struct{}

func (UnixSignaler) Stop(pid int) error {
	return syscall.Kill(pid, syscall.SIGSTOP)
}

func (UnixSignaler) Continue(pid int) error {
	return syscall.Kill(pid, syscall.SIGCONT)
}

func (UnixSignaler) Name(pid int) (string, error) {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return "", err
	}
	if len(raw) == 0 {
		return "", nil
	}
	if raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	return string(raw), nil
}

type Rec struct {
	Stops     []int
	Continues []int
	Locks     int
}

type Recorder struct {
	Rec      *Rec
	Inner    Signaler
	AllowPID map[int]bool
	Names    map[int]string
}

func (r *Recorder) Stop(pid int) error {
	if r.AllowPID != nil && !r.AllowPID[pid] {
		return fmt.Errorf("refusing to signal pid %d", pid)
	}
	r.Rec.Stops = append(r.Rec.Stops, pid)
	if r.Inner != nil {
		return r.Inner.Stop(pid)
	}
	return nil
}

func (r *Recorder) Continue(pid int) error {
	if r.AllowPID != nil && !r.AllowPID[pid] {
		return fmt.Errorf("refusing to signal pid %d", pid)
	}
	r.Rec.Continues = append(r.Rec.Continues, pid)
	if r.Inner != nil {
		return r.Inner.Continue(pid)
	}
	return nil
}

func (r *Recorder) Name(pid int) (string, error) {
	if r.Names != nil {
		if n, ok := r.Names[pid]; ok {
			return n, nil
		}
	}
	if r.Inner != nil {
		return r.Inner.Name(pid)
	}
	return "toy", nil
}

type CountingLocker struct {
	Rec *Rec
}

func (l *CountingLocker) Lock() error {
	l.Rec.Locks++
	return nil
}

type StaticFocus struct {
	W   Window
	OK  bool
	Err error
}

func (f *StaticFocus) Active() (Window, bool, error) {
	return f.W, f.OK, f.Err
}

type StaticSession struct {
	IsLocked bool
	IsIdle   bool
}

func (s StaticSession) Locked() bool { return s.IsLocked }
func (s StaticSession) Idle() bool   { return s.IsIdle }

type Enforcer struct {
	Bank       *bank.Bank
	Focus      FocusSource
	Session    Session
	Signals    Signaler
	Locker     Locker
	SessionUID func() int
}

func (e *Enforcer) Tick() error {
	if e.SessionUID != nil {
		_ = e.SessionUID()
	}
	if e.Bank.OverlayActive() {
		e.Bank.SetFocus("", "")
		return nil
	}
	cfg := e.Bank.Config()
	if e.Session != nil && (e.Session.Locked() || e.Session.Idle()) {
		return nil
	}
	win, ok, err := e.Focus.Active()
	if err != nil || !ok {
		return nil
	}
	if cfg.AlwaysOnClass(win.Class) {
		e.Bank.SetFocus("", "")
		return nil
	}
	if cfg.Lab.Enabled && !cfg.LabClass(win.Class) {
		e.Bank.SetFocus("", "")
		return nil
	}
	hit := e.Bank.FocusHit(win.Class, win.Title)
	if !hit.Matched {
		e.Bank.SetFocus("", "")
		return nil
	}
	e.Bank.SetFocus(hit.Pile, hit.App)
	label := hit.App
	if label == "" {
		label = win.Class
	}
	if err := e.Bank.NoteToday(label); err != nil {
		return err
	}
	if hit.Pile == "" {
		return nil
	}
	res, err := e.Bank.Spend(hit.Pile)
	if err != nil {
		return err
	}
	if res.Kind == bank.SpendTick {
		if err := e.Bank.AddSpent("fun", 1); err != nil {
			return err
		}
		_, paused, err := e.Bank.IsPaused(win.PID)
		if err != nil {
			return err
		}
		if paused {
			return e.cont(win.PID)
		}
		return nil
	}
	if res.Kind == bank.SpendEmpty {
		return e.stop(win.PID, hit.Pile)
	}
	return nil
}

func (e *Enforcer) Resume(credited string) error {
	paused, err := e.Bank.PausedToResume(credited)
	if err != nil {
		return err
	}
	for _, p := range paused {
		if err := e.cont(p.PID); err != nil {
			return err
		}
	}
	return nil
}

func (e *Enforcer) stop(pid int, group string) error {
	cfg := e.Bank.Config()
	if pid > 0 {
		name, err := e.Signals.Name(pid)
		if err == nil && cfg.NeverSignalName(name) {
			return nil
		}
		if cfg.Enforcer {
			if err := e.Signals.Stop(pid); err != nil {
				return err
			}
		}
	}
	return e.Bank.RecordPaused(pid, group)
}

func (e *Enforcer) cont(pid int) error {
	cfg := e.Bank.Config()
	if pid > 0 {
		name, err := e.Signals.Name(pid)
		if err == nil && cfg.NeverSignalName(name) {
			return nil
		}
		if cfg.Enforcer {
			if err := e.Signals.Continue(pid); err != nil {
				return err
			}
		}
	}
	return e.Bank.ClearPaused(pid)
}

func (e *Enforcer) lockSession(uid int) error {
	if e.SessionUID != nil && uid <= 0 {
		return nil
	}
	if e.Session != nil && e.Session.Locked() {
		return nil
	}
	if e.Locker == nil {
		return nil
	}
	return e.Locker.Lock()
}

type MutableSession struct {
	LockedHint bool
	IdleHint   bool
}

func (s *MutableSession) Locked() bool { return s.LockedHint }
func (s *MutableSession) Idle() bool   { return s.IdleHint }
