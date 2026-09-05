package reverse

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"kidtimer/daemon/internal/netaddr"
)

var (
	ErrFramePayload = errors.New("frame: exactly one payload")
	ErrFrameKind    = errors.New("frame: kind mismatch")
)

type wireFrame struct {
	Kind     string    `json:"kind"`
	Offer    *Offer    `json:"offer,omitempty"`
	Resume   *Resume   `json:"resume,omitempty"`
	Accept   *Accept   `json:"accept,omitempty"`
	ResumeOK *ResumeOK `json:"resume_ok,omitempty"`
	Reject   *Reject   `json:"reject,omitempty"`
	Op       *Op       `json:"op,omitempty"`
	Result   *OpResult `json:"result,omitempty"`
}

func ParseFrame(raw []byte) (Frame, error) {
	var w wireFrame
	if err := json.Unmarshal(raw, &w); err != nil {
		return Frame{}, err
	}
	n := 0
	if w.Offer != nil {
		n++
	}
	if w.Resume != nil {
		n++
	}
	if w.Accept != nil {
		n++
	}
	if w.ResumeOK != nil {
		n++
	}
	if w.Reject != nil {
		n++
	}
	if w.Op != nil {
		n++
	}
	if w.Result != nil {
		n++
	}
	if n != 1 {
		return Frame{}, ErrFramePayload
	}
	f := Frame{
		Offer:    w.Offer,
		Resume:   w.Resume,
		Accept:   w.Accept,
		ResumeOK: w.ResumeOK,
		Reject:   w.Reject,
		Op:       w.Op,
		Result:   w.Result,
	}
	want, ok := kindFromPayload(f)
	if !ok {
		return Frame{}, ErrFramePayload
	}
	if w.Kind != "" && w.Kind != kindName(want) {
		return Frame{}, ErrFrameKind
	}
	f.Kind = want
	return f, nil
}

func EncodeFrame(f Frame) ([]byte, error) {
	kind, ok := kindFromPayload(f)
	if !ok {
		return nil, ErrFramePayload
	}
	w := wireFrame{
		Kind:     kindName(kind),
		Offer:    f.Offer,
		Resume:   f.Resume,
		Accept:   f.Accept,
		ResumeOK: f.ResumeOK,
		Reject:   f.Reject,
		Op:       f.Op,
		Result:   f.Result,
	}
	return json.Marshal(w)
}

func kindFromPayload(f Frame) (Kind, bool) {
	n := 0
	var k Kind
	if f.Offer != nil {
		n++
		k = KindOffer
	}
	if f.Resume != nil {
		n++
		k = KindResume
	}
	if f.Accept != nil {
		n++
		k = KindAccept
	}
	if f.ResumeOK != nil {
		n++
		k = KindResumeOK
	}
	if f.Reject != nil {
		n++
		k = KindReject
	}
	if f.Op != nil {
		n++
		k = KindOp
	}
	if f.Result != nil {
		n++
		k = KindResult
	}
	return k, n == 1
}

func kindName(k Kind) string {
	switch k {
	case KindOffer:
		return "offer"
	case KindResume:
		return "resume"
	case KindAccept:
		return "accept"
	case KindResumeOK:
		return "resume_ok"
	case KindReject:
		return "reject"
	case KindOp:
		return "op"
	case KindResult:
		return "result"
	default:
		return ""
	}
}

func HashTicket(t Ticket) TicketHash {
	sum := sha256.Sum256([]byte(t))
	return TicketHash(hex.EncodeToString(sum[:]))
}

func NewTicket() (Ticket, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return Ticket(hex.EncodeToString(raw)), nil
}

func IsSessionPeer(ip net.IP) bool {
	return netaddr.IsHousehold(ip)
}

func WriteFrame(w io.Writer, f Frame) error {
	raw, err := EncodeFrame(f)
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}

func ReadFrame(r *bufio.Reader) (Frame, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return Frame{}, err
	}
	return ParseFrame(bytes.TrimSpace(line))
}

type Handler interface {
	OnOffer(Offer) (*Accept, *Reject)
	OnResume(Resume) (*ResumeOK, *Reject)
	OnAccept(Accept) error
	OnResumeOK() error
	OnReject(Reject) error
	OnOp(Op) OpResult
	OnResult(OpResult)
	OnDrop()
}

func ServeConn(ctx context.Context, c net.Conn, h Handler) error {
	defer h.OnDrop()
	br := bufio.NewReader(c)
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		f, err := ReadFrame(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch f.Kind {
		case KindOffer:
			acc, rej := h.OnOffer(*f.Offer)
			if rej != nil {
				return WriteFrame(c, Frame{Reject: rej})
			}
			if acc != nil {
				if err := WriteFrame(c, Frame{Accept: acc}); err != nil {
					return err
				}
			}
		case KindResume:
			ok, rej := h.OnResume(*f.Resume)
			if rej != nil {
				return WriteFrame(c, Frame{Reject: rej})
			}
			if ok != nil {
				if err := WriteFrame(c, Frame{ResumeOK: ok}); err != nil {
					return err
				}
			}
		case KindOp:
			res := h.OnOp(*f.Op)
			if err := WriteFrame(c, Frame{Result: &res}); err != nil {
				return err
			}
		case KindResult:
			h.OnResult(*f.Result)
		case KindAccept:
			if err := h.OnAccept(*f.Accept); err != nil {
				return err
			}
		case KindResumeOK:
			if err := h.OnResumeOK(); err != nil {
				return err
			}
		case KindReject:
			return h.OnReject(*f.Reject)
		default:
			return fmt.Errorf("unexpected frame %v", f.Kind)
		}
	}
}

func OpenConn(ctx context.Context, ep Endpoint, hello Frame, h Handler) error {
	d := net.Dialer{Timeout: 2 * time.Second}
	c, err := d.DialContext(ctx, "tcp", string(ep))
	if err != nil {
		return err
	}
	defer c.Close()
	if err := WriteFrame(c, hello); err != nil {
		return err
	}
	return ServeConn(ctx, c, h)
}
