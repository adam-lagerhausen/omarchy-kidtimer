package enforcer

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	wlDisplayID              = 1
	wlDisplaySync            = 0
	wlDisplayGetRegistry     = 1
	wlDisplayError           = 0
	wlDisplayDeleteID        = 1
	wlRegistryBind           = 0
	wlRegistryGlobal         = 0
	wlCallbackDone           = 0
	idleNotifierGetInputIdle = 2
	idleNotificationIdled    = 0
	idleNotificationResumed  = 1

	wlSeatIface        = "wl_seat"
	idleNotifierIface  = "ext_idle_notifier_v1"
	idleNotifierMinVer = 2
)

type wlMsg struct {
	id     uint32
	opcode uint32
	body   []byte
}

func encodeMsg(id, opcode uint32, body []byte) []byte {
	size := 8 + len(body)
	out := make([]byte, size)
	binary.LittleEndian.PutUint32(out[0:4], id)
	binary.LittleEndian.PutUint32(out[4:8], uint32(size)<<16|opcode)
	copy(out[8:], body)
	return out
}

func readMsg(r io.Reader) (wlMsg, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return wlMsg{}, err
	}
	id := binary.LittleEndian.Uint32(hdr[0:4])
	sizeOp := binary.LittleEndian.Uint32(hdr[4:8])
	size := sizeOp >> 16
	opcode := sizeOp & 0xffff
	if size < 8 || size > 4096 {
		return wlMsg{}, fmt.Errorf("wayland: bad size %d", size)
	}
	msg := wlMsg{id: id, opcode: opcode}
	if size > 8 {
		msg.body = make([]byte, size-8)
		if _, err := io.ReadFull(r, msg.body); err != nil {
			return wlMsg{}, err
		}
	}
	return msg, nil
}

func putU32(buf []byte, v uint32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return append(buf, b[:]...)
}

func putString(buf []byte, s string) []byte {
	n := len(s) + 1
	buf = putU32(buf, uint32(n))
	buf = append(buf, s...)
	buf = append(buf, 0)
	for len(buf)%4 != 0 {
		buf = append(buf, 0)
	}
	return buf
}

func getU32(b []byte, off int) (uint32, int, bool) {
	if off+4 > len(b) {
		return 0, off, false
	}
	return binary.LittleEndian.Uint32(b[off : off+4]), off + 4, true
}

func getString(b []byte, off int) (string, int, bool) {
	n, off, ok := getU32(b, off)
	if !ok {
		return "", off, false
	}
	if n == 0 {
		return "", (off + 3) &^ 3, true
	}
	end := off + int(n)
	if end > len(b) {
		return "", off, false
	}
	s := string(b[off : end-1])
	return s, (end + 3) &^ 3, true
}
