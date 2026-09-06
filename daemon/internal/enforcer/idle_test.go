package enforcer

import (
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIdleDoesNotUseLogindHint(t *testing.T) {
	b, err := os.ReadFile("hyprland.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "IdleHint") {
		t.Fatal("IdleHint does not ignore leftover games")
	}
	idle, err := os.ReadFile("idle.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(idle), "get_input_idle_notification") {
		t.Fatal("must use input idle, not the inhibitor-obeying request")
	}
}

func TestLogindSessionNoUIDIsIdle(t *testing.T) {
	var s *LogindSession
	if !s.Idle() {
		t.Fatal("nil session")
	}
	s = &LogindSession{Watch: &InputIdle{}}
	if !s.Idle() {
		t.Fatal("uid 0")
	}
}

func TestInputIdleMissingProtocolDoesNotSpend(t *testing.T) {
	e, _, _ := setup(t, false, afternoon)
	e.Focus = &StaticFocus{W: Window{Class: "PrismLauncher", PID: 7}, OK: true}
	e.Session = &LogindSession{UID: 42, Watch: &InputIdle{
		Dial: func(int) (net.Conn, error) { return nil, errNoInputIdle },
	}}
	before := remaining(t, e, "fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != before {
		t.Fatal("spent without an idle source")
	}
}

func TestIdleClearsFocus(t *testing.T) {
	e, _, parent := setup(t, false, afternoon)
	e.Bank.SetFocus("fun", "minecraft")
	e.Session = StaticSession{IsIdle: true}
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	st, err := e.Bank.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if st.FocusedGroup != "" || st.FocusedApp != "" {
		t.Fatalf("idle must clear focus: group=%q app=%q", st.FocusedGroup, st.FocusedApp)
	}
}

func TestInputIdleRequestsSixtySeconds(t *testing.T) {
	srv := startFakeIdle(t, idleNotifierMinVer)
	w := &InputIdle{Dial: srv.dial}
	t.Cleanup(func() { w.Attach(0) })
	w.Attach(1000)
	srv.waitBound(t)
	waitUntil(t, func() bool { return !w.Idle() })
	if srv.timeoutMs() != uint32(inputIdleTimeout/time.Millisecond) {
		t.Fatalf("timeout %d", srv.timeoutMs())
	}
}

func TestInputIdleIdledAndResumed(t *testing.T) {
	srv := startFakeIdle(t, idleNotifierMinVer)
	w := &InputIdle{Dial: srv.dial}
	t.Cleanup(func() { w.Attach(0) })
	w.Attach(1000)
	srv.waitBound(t)
	waitUntil(t, func() bool { return !w.Idle() })
	if err := srv.sendNote(idleNotificationIdled); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return w.Idle() })
	if err := srv.sendNote(idleNotificationResumed); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return !w.Idle() })
}

func TestInputIdleV1ProtocolStaysIdle(t *testing.T) {
	srv := startFakeIdle(t, 1)
	w := &InputIdle{Dial: srv.dial}
	t.Cleanup(func() { w.Attach(0) })
	w.Attach(1000)
	select {
	case <-srv.bound:
		t.Fatal("v1 must not bind get_input_idle_notification")
	case <-time.After(200 * time.Millisecond):
	}
	if !w.Idle() {
		t.Fatal("missing v2 must not spend")
	}
}

func TestInputIdleLeftoverWindowDoesNotSpend(t *testing.T) {
	srv := startFakeIdle(t, idleNotifierMinVer)
	w := &InputIdle{Dial: srv.dial}
	t.Cleanup(func() { w.Attach(0) })
	e, _, _ := setup(t, false, afternoon)
	e.Focus = &StaticFocus{W: Window{Class: "PrismLauncher", PID: 7}, OK: true}
	e.Session = &LogindSession{UID: 1000, Watch: w}
	w.Attach(1000)
	srv.waitBound(t)
	waitUntil(t, func() bool { return !w.Idle() })
	before := remaining(t, e, "fun")
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != before-1 {
		t.Fatal("hands on the computer must spend")
	}
	if err := srv.sendNote(idleNotificationIdled); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, func() bool { return w.Idle() })
	if err := e.Tick(); err != nil {
		t.Fatal(err)
	}
	if remaining(t, e, "fun") != before-1 {
		t.Fatal("leftover window spent after idle")
	}
}

type fakeIdle struct {
	ln      net.Listener
	version uint32
	bound   chan struct{}
	once    sync.Once

	mu      sync.Mutex
	conn    net.Conn
	noteID  uint32
	timeout uint32
}

func startFakeIdle(t *testing.T, version uint32) *fakeIdle {
	t.Helper()
	ln, err := net.Listen("unix", t.TempDir()+"/wayland-0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdle{ln: ln, version: version, bound: make(chan struct{})}
	go f.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		f.mu.Lock()
		if f.conn != nil {
			_ = f.conn.Close()
		}
		f.mu.Unlock()
	})
	return f
}

func (f *fakeIdle) dial(int) (net.Conn, error) {
	return net.Dial("unix", f.ln.Addr().String())
}

func (f *fakeIdle) timeoutMs() uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.timeout
}

func (f *fakeIdle) waitBound(t *testing.T) {
	t.Helper()
	select {
	case <-f.bound:
	case <-time.After(2 * time.Second):
		t.Fatal("idle notification not bound")
	}
}

func (f *fakeIdle) sendNote(opcode uint32) error {
	f.mu.Lock()
	id := f.noteID
	conn := f.conn
	f.mu.Unlock()
	if conn == nil || id == 0 {
		return io.ErrClosedPipe
	}
	return f.write(encodeMsg(id, opcode, nil))
}

func (f *fakeIdle) write(msg []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.conn == nil {
		return io.ErrClosedPipe
	}
	_, err := f.conn.Write(msg)
	return err
}

func (f *fakeIdle) serve() {
	conn, err := f.ln.Accept()
	if err != nil {
		return
	}
	f.mu.Lock()
	f.conn = conn
	f.mu.Unlock()
	defer conn.Close()

	var registryID uint32
	ifaceOf := map[uint32]string{}
	for {
		msg, err := readMsg(conn)
		if err != nil {
			return
		}
		switch {
		case msg.id == wlDisplayID && msg.opcode == wlDisplayGetRegistry:
			registryID, _, _ = getU32(msg.body, 0)
			_ = f.write(encodeMsg(registryID, wlRegistryGlobal, globalBody(1, wlSeatIface, 8)))
			_ = f.write(encodeMsg(registryID, wlRegistryGlobal, globalBody(2, idleNotifierIface, f.version)))
		case msg.id == wlDisplayID && msg.opcode == wlDisplaySync:
			cb, _, _ := getU32(msg.body, 0)
			_ = f.write(encodeMsg(cb, wlCallbackDone, putU32(nil, 0)))
			_ = f.write(encodeMsg(wlDisplayID, wlDisplayDeleteID, putU32(nil, cb)))
		case msg.id == registryID && msg.opcode == wlRegistryBind:
			_, off, _ := getU32(msg.body, 0)
			iface, off, _ := getString(msg.body, off)
			_, off, _ = getU32(msg.body, off)
			id, _, _ := getU32(msg.body, off)
			ifaceOf[id] = iface
		case ifaceOf[msg.id] == idleNotifierIface && msg.opcode == idleNotifierGetInputIdle:
			note, off, _ := getU32(msg.body, 0)
			timeout, _, _ := getU32(msg.body, off)
			f.mu.Lock()
			f.noteID = note
			f.timeout = timeout
			f.mu.Unlock()
			f.once.Do(func() { close(f.bound) })
		}
	}
}

func globalBody(name uint32, iface string, version uint32) []byte {
	buf := putU32(nil, name)
	buf = putString(buf, iface)
	return putU32(buf, version)
}

func waitUntil(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timeout")
}
