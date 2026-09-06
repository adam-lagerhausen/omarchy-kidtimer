package enforcer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const inputIdleTimeout = 60 * time.Second

var errNoInputIdle = errors.New("ext_idle_notifier_v1 v2 missing")

// InputIdle watches Hyprland seat activity through
// ext_idle_notifier_v1.get_input_idle_notification. That request ignores
// screensaver inhibitors, so a leftover fullscreen game still goes idle.
type InputIdle struct {
	Dial func(uid int) (net.Conn, error)

	mu      sync.Mutex
	uid     int
	running bool
	cancel  context.CancelFunc
	ready   atomic.Bool
	idle    atomic.Bool
}

func (w *InputIdle) Idle() bool {
	if w == nil || !w.ready.Load() {
		return true
	}
	return w.idle.Load()
}

func (w *InputIdle) Attach(uid int) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if uid <= 0 {
		w.stopLocked()
		return
	}
	if w.uid == uid && w.running {
		return
	}
	w.stopLocked()
	w.uid = uid
	w.running = true
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	go w.loop(ctx, uid)
}

func (w *InputIdle) stopLocked() {
	if w.cancel != nil {
		w.cancel()
		w.cancel = nil
	}
	w.running = false
	w.uid = 0
	w.ready.Store(false)
	w.idle.Store(false)
}

func (w *InputIdle) loop(ctx context.Context, uid int) {
	defer func() {
		w.mu.Lock()
		if w.uid == uid {
			w.running = false
			w.ready.Store(false)
			w.idle.Store(false)
		}
		w.mu.Unlock()
	}()
	conn, err := w.dial(uid)
	if err != nil {
		return
	}
	defer conn.Close()
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	err = w.run(ctx, conn)
	if errors.Is(err, errNoInputIdle) {
		<-ctx.Done()
	}
}

func (w *InputIdle) dial(uid int) (net.Conn, error) {
	if w.Dial != nil {
		return w.Dial(uid)
	}
	return dialWayland(uid)
}

func (w *InputIdle) timeoutMs() uint32 {
	return uint32(inputIdleTimeout / time.Millisecond)
}

func dialWayland(uid int) (net.Conn, error) {
	if uid <= 0 {
		return nil, errNoInputIdle
	}
	runtimeDir := fmt.Sprintf("/run/user/%d", uid)
	display := waylandDisplay(runtimeDir)
	if display == "" {
		return nil, errNoInputIdle
	}
	return net.DialTimeout("unix", filepath.Join(runtimeDir, display), 2*time.Second)
}

func (w *InputIdle) run(ctx context.Context, conn net.Conn) error {
	c := &wlClient{conn: conn, next: 2}
	registryID := c.alloc()
	if err := c.send(wlDisplayID, wlDisplayGetRegistry, putU32(nil, registryID)); err != nil {
		return err
	}
	callbackID := c.alloc()
	if err := c.send(wlDisplayID, wlDisplaySync, putU32(nil, callbackID)); err != nil {
		return err
	}

	var seatName, idleName, idleVer uint32
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		msg, err := readMsg(conn)
		if err != nil {
			return err
		}
		if msg.id == wlDisplayID && msg.opcode == wlDisplayError {
			return fmt.Errorf("wayland display error")
		}
		if msg.id == registryID && msg.opcode == wlRegistryGlobal {
			name, off, ok := getU32(msg.body, 0)
			iface, off, ok2 := getString(msg.body, off)
			ver, _, ok3 := getU32(msg.body, off)
			if !ok || !ok2 || !ok3 {
				continue
			}
			switch iface {
			case wlSeatIface:
				if seatName == 0 {
					seatName = name
				}
			case idleNotifierIface:
				idleName, idleVer = name, ver
			}
			continue
		}
		if msg.id == callbackID && msg.opcode == wlCallbackDone {
			break
		}
	}
	if seatName == 0 || idleVer < idleNotifierMinVer {
		return errNoInputIdle
	}

	seatID := c.alloc()
	if err := c.send(registryID, wlRegistryBind, marshalBind(seatName, wlSeatIface, 1, seatID)); err != nil {
		return err
	}
	idleID := c.alloc()
	if err := c.send(registryID, wlRegistryBind, marshalBind(idleName, idleNotifierIface, idleNotifierMinVer, idleID)); err != nil {
		return err
	}
	noteID := c.alloc()
	body := putU32(nil, noteID)
	body = putU32(body, w.timeoutMs())
	body = putU32(body, seatID)
	if err := c.send(idleID, idleNotifierGetInputIdle, body); err != nil {
		return err
	}
	w.idle.Store(false)
	w.ready.Store(true)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		msg, err := readMsg(conn)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if msg.id == wlDisplayID && msg.opcode == wlDisplayError {
			return fmt.Errorf("wayland display error")
		}
		if msg.id != noteID {
			continue
		}
		switch msg.opcode {
		case idleNotificationIdled:
			w.idle.Store(true)
		case idleNotificationResumed:
			w.idle.Store(false)
		}
	}
}

func marshalBind(name uint32, iface string, version, id uint32) []byte {
	buf := putU32(nil, name)
	buf = putString(buf, iface)
	buf = putU32(buf, version)
	return putU32(buf, id)
}

type wlClient struct {
	conn net.Conn
	next uint32
}

func (c *wlClient) alloc() uint32 {
	id := c.next
	c.next++
	return id
}

func (c *wlClient) send(id, opcode uint32, body []byte) error {
	_, err := c.conn.Write(encodeMsg(id, opcode, body))
	return err
}

func (s *LogindSession) Idle() bool {
	if s == nil || s.UID <= 0 {
		if s != nil && s.Watch != nil {
			s.Watch.Attach(0)
		}
		return true
	}
	if s.Watch == nil {
		return true
	}
	s.Watch.Attach(s.UID)
	return s.Watch.Idle()
}
