package desk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"kidtimer/daemon/internal/advertise"
	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/netaddr"
	"kidtimer/daemon/internal/reverse"
)

const (
	DefaultHTTP    = "127.0.0.1:8741"
	DefaultSession = "0.0.0.0:8743"
)

type Config struct {
	Home        string
	HTTPAddr    string
	SessionAddr string
	SkipScan    bool
	OnListen    func(httpAddr, sessionAddr string)
}

type TestConfig = Config

var errOffline = errors.New("offline")

type Registry struct {
	mu     sync.Mutex
	path   string
	links  map[reverse.KidID]*link
	corr   atomic.Uint64
	browse func(ctx context.Context, out chan<- advertise.Found) error
	probe  func(ctx context.Context) []string
	pair   func(ctx context.Context, url string) (pairResult, error)
}

type link struct {
	mu      sync.Mutex
	wmu     sync.Mutex
	conn    net.Conn
	pending map[uint64]chan reverse.OpResult
	live    bool
}

func NewRegistry(path string) *Registry {
	return &Registry{path: path, links: map[reverse.KidID]*link{}}
}

func (r *Registry) Household(records []reverse.Record) reverse.Household {
	out := make([]reverse.Member, 0, len(records))
	for _, rec := range records {
		m := reverse.Member{ID: rec.ID, Name: rec.Name, Live: r.isLive(rec.ID)}
		if res, _ := r.Call(rec.ID, reverse.Op{Method: http.MethodGet, Path: "/v1/status"}); res.Status == 200 {
			m.Live = true
			m.Status = res.Body
		}
		if res, _ := r.Call(rec.ID, reverse.Op{Method: http.MethodGet, Path: "/v1/asks"}); res.Status == 200 {
			m.Asks = asksArray(res.Body)
		}
		if res, _ := r.Call(rec.ID, reverse.Op{Method: http.MethodGet, Path: "/v1/look"}); res.Status == 200 {
			m.Look = res.Body
		}
		out = append(out, m)
	}
	return reverse.Household{Kids: out}
}

func asksArray(raw json.RawMessage) json.RawMessage {
	var wrap struct {
		Asks json.RawMessage `json:"asks"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil && wrap.Asks != nil {
		return wrap.Asks
	}
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	return raw
}

func (r *Registry) isLive(id reverse.KidID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	l := r.links[id]
	return l != nil && l.live
}

func (r *Registry) Call(id reverse.KidID, op reverse.Op) (reverse.OpResult, error) {
	r.mu.Lock()
	l := r.links[id]
	r.mu.Unlock()
	if l != nil && l.live {
		return r.callSession(l, op)
	}
	return r.callHTTP(id, op)
}

func (r *Registry) callSession(l *link, op reverse.Op) (reverse.OpResult, error) {
	if op.Corr == 0 {
		op.Corr = r.corr.Add(1)
	}
	ch := make(chan reverse.OpResult, 1)
	l.mu.Lock()
	if l.pending == nil {
		l.pending = map[uint64]chan reverse.OpResult{}
	}
	l.pending[op.Corr] = ch
	l.mu.Unlock()
	l.wmu.Lock()
	err := reverse.WriteFrame(l.conn, reverse.Frame{Op: &op})
	l.wmu.Unlock()
	if err != nil {
		return reverse.OpResult{}, err
	}
	select {
	case res := <-ch:
		return res, nil
	case <-time.After(5 * time.Second):
		return reverse.OpResult{}, fmt.Errorf("op timeout")
	}
}

func (r *Registry) record(id reverse.KidID) (reverse.Record, bool) {
	kids, err := household.Load(r.path)
	if err != nil {
		return reverse.Record{}, false
	}
	return household.Lookup(kids, id)
}

func (r *Registry) callHTTP(id reverse.KidID, op reverse.Op) (reverse.OpResult, error) {
	rec, ok := r.record(id)
	if !ok || rec.URL == "" || rec.Token == "" {
		return reverse.OpResult{Corr: op.Corr, Status: http.StatusServiceUnavailable, Body: json.RawMessage(`{"error":"offline"}`)}, errOffline
	}
	return httpOp(rec.URL, rec.Token, op)
}

func httpOp(base, token string, op reverse.Op) (reverse.OpResult, error) {
	u := strings.TrimRight(base, "/") + op.Path
	var body io.Reader
	if len(op.Body) > 0 {
		body = bytes.NewReader(op.Body)
	}
	req, err := http.NewRequest(op.Method, u, body)
	if err != nil {
		return reverse.OpResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if op.IdemKey != "" {
		req.Header.Set("Idempotency-Key", op.IdemKey)
	}
	if len(op.Body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return reverse.OpResult{Corr: op.Corr, Status: http.StatusServiceUnavailable, Body: json.RawMessage(`{"error":"offline"}`)}, errOffline
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	return reverse.OpResult{Corr: op.Corr, Status: resp.StatusCode, Body: raw}, nil
}

func (r *Registry) attach(id reverse.KidID, c net.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.links[id] = &link{conn: c, live: true, pending: map[uint64]chan reverse.OpResult{}}
}

func (r *Registry) pushPin(id reverse.KidID) {
	hash, ok, err := household.ReadPin(filepath.Dir(r.path))
	if err != nil || !ok {
		return
	}
	body, err := json.Marshal(map[string]string{"hash": hash})
	if err != nil {
		return
	}
	_, _ = r.Call(id, reverse.Op{Method: http.MethodPut, Path: "/v1/parent-pin", Body: body})
}

func (r *Registry) detach(id reverse.KidID) {
	if id == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.links[id]; ok {
		l.live = false
	}
}

func (r *Registry) deliver(res reverse.OpResult) {
	r.mu.Lock()
	var hit *link
	for _, l := range r.links {
		l.mu.Lock()
		_, ok := l.pending[res.Corr]
		l.mu.Unlock()
		if ok {
			hit = l
			break
		}
	}
	r.mu.Unlock()
	if hit == nil {
		return
	}
	hit.mu.Lock()
	ch := hit.pending[res.Corr]
	delete(hit.pending, res.Corr)
	hit.mu.Unlock()
	if ch != nil {
		ch <- res
	}
}

func (r *Registry) acceptOffer(o reverse.Offer) (*reverse.Accept, *reverse.Reject) {
	if o.ID == "" {
		return nil, &reverse.Reject{Reason: reverse.RejectBadTicket}
	}
	kids, err := household.Load(r.path)
	if err != nil {
		return nil, &reverse.Reject{Reason: reverse.RejectBadTicket}
	}
	if _, ok := household.Lookup(kids, o.ID); ok {
		return nil, &reverse.Reject{Reason: reverse.RejectBadTicket}
	}
	ticket, err := reverse.NewTicket()
	if err != nil {
		return nil, &reverse.Reject{Reason: reverse.RejectBadTicket}
	}
	row := reverse.Record{
		ID:         o.ID,
		Name:       o.Name,
		TicketHash: reverse.HashTicket(ticket),
		PairedAt:   time.Now().UTC(),
	}
	if err := household.Save(r.path, household.Upsert(kids, row)); err != nil {
		return nil, &reverse.Reject{Reason: reverse.RejectBadTicket}
	}
	return &reverse.Accept{Ticket: ticket}, nil
}

func (r *Registry) acceptResume(s reverse.Resume) (*reverse.ResumeOK, *reverse.Reject) {
	kids, err := household.Load(r.path)
	if err != nil {
		return nil, &reverse.Reject{Reason: reverse.RejectBadTicket}
	}
	rec, ok := household.Lookup(kids, s.ID)
	if !ok || rec.TicketHash != reverse.HashTicket(s.Ticket) {
		return nil, &reverse.Reject{Reason: reverse.RejectBadTicket}
	}
	return &reverse.ResumeOK{}, nil
}

type sessionHandler struct {
	r    *Registry
	conn net.Conn
	id   reverse.KidID
}

func (h *sessionHandler) OnOffer(o reverse.Offer) (*reverse.Accept, *reverse.Reject) {
	acc, rej := h.r.acceptOffer(o)
	if rej != nil {
		return nil, rej
	}
	h.id = o.ID
	h.r.attach(o.ID, h.conn)
	go h.r.pushPin(o.ID)
	return acc, nil
}

func (h *sessionHandler) OnResume(s reverse.Resume) (*reverse.ResumeOK, *reverse.Reject) {
	ok, rej := h.r.acceptResume(s)
	if rej != nil {
		return nil, rej
	}
	h.id = s.ID
	h.r.attach(s.ID, h.conn)
	go h.r.pushPin(s.ID)
	return ok, nil
}

func (h *sessionHandler) OnAccept(reverse.Accept) error { return nil }
func (h *sessionHandler) OnResumeOK() error             { return nil }
func (h *sessionHandler) OnReject(reverse.Reject) error {
	return fmt.Errorf("parent received reject")
}
func (h *sessionHandler) OnOp(op reverse.Op) reverse.OpResult {
	return reverse.OpResult{Corr: op.Corr, Status: http.StatusBadRequest, Body: json.RawMessage(`{"error":"parent does not serve ops"}`)}
}
func (h *sessionHandler) OnResult(res reverse.OpResult) { h.r.deliver(res) }
func (h *sessionHandler) OnDrop()                       { h.r.detach(h.id) }

func Run(ctx context.Context, cfg Config) error {
	if cfg.Home == "" {
		return fmt.Errorf("desk: home is required")
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = DefaultHTTP
	}
	if cfg.SessionAddr == "" {
		cfg.SessionAddr = DefaultSession
	}
	path := household.Path(cfg.Home)
	if err := os.MkdirAll(cfg.Home, 0o700); err != nil {
		return err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := household.Save(path, nil); err != nil {
			return err
		}
	}
	r := NewRegistry(path)
	httpLn, err := netaddr.TryListen(cfg.HTTPAddr)
	if err != nil {
		return err
	}
	sessLn, err := netaddr.TryListen(cfg.SessionAddr)
	if err != nil {
		_ = httpLn.Close()
		return err
	}
	if cfg.OnListen != nil {
		cfg.OnListen(boundLoopback(httpLn), boundLoopback(sessLn))
	}

	if !cfg.SkipScan {
		go r.scanLoop(ctx)
	}

	srv := &http.Server{Handler: loopbackOnly(http.HandlerFunc(r.serveHTTP))}
	errCh := make(chan error, 2)
	go func() { errCh <- srv.Serve(httpLn) }()
	go func() { errCh <- serveSession(ctx, sessLn, r) }()

	if port, err := advertise.PortOf(sessLn.Addr().String()); err == nil {
		id, idErr := advertise.MachineID(filepath.Join(cfg.Home, "machine-id"))
		name, _ := os.Hostname()
		if name == "" {
			name = "kidtimer"
		}
		if idErr == nil {
			if svc, aerr := advertise.RegisterService(name, advertise.ServiceParent, id, port); aerr == nil {
				defer svc.Shutdown()
			}
		}
	}

	select {
	case <-ctx.Done():
		_ = srv.Close()
		_ = sessLn.Close()
		return nil
	case err := <-errCh:
		_ = srv.Close()
		_ = sessLn.Close()
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
}

func boundLoopback(ln net.Listener) string {
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		return ln.Addr().String()
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func loopbackOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if netaddr.Classify(netaddr.PeerIP(r.RemoteAddr)) != netaddr.ClassLoopback {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "loopback only"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func serveSession(ctx context.Context, ln net.Listener, r *Registry) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		c, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		ip := netaddr.PeerIP(c.RemoteAddr().String())
		if !reverse.IsSessionPeer(ip) {
			_ = c.Close()
			continue
		}
		go func(c net.Conn) {
			h := &sessionHandler{r: r, conn: c}
			_ = reverse.ServeConn(ctx, c, h)
			_ = c.Close()
		}(c)
	}
}

func (r *Registry) serveHTTP(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/v1/household" && req.Method == http.MethodGet {
		kids, err := household.Load(r.path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if kids == nil {
			kids = []reverse.Record{}
		}
		writeJSON(w, http.StatusOK, r.Household(kids))
		return
	}
	id, rest, ok := kidPath(req.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	op := reverse.Op{Method: req.Method, Path: rest, IdemKey: req.Header.Get("Idempotency-Key")}
	if req.Body != nil && req.Method != http.MethodGet {
		body, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		if len(body) > 0 {
			op.Body = body
		}
	}
	res, err := r.Call(reverse.KidID(id), op)
	if err != nil && !errors.Is(err, errOffline) {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if res.Status == 0 {
		res.Status = http.StatusOK
	}
	w.WriteHeader(res.Status)
	if len(res.Body) > 0 {
		_, _ = w.Write(res.Body)
	} else {
		_, _ = w.Write([]byte("{}\n"))
	}
}

func kidPath(path string) (id, rest string, ok bool) {
	const prefix = "/v1/kids/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	tail := strings.TrimPrefix(path, prefix)
	id, after, found := strings.Cut(tail, "/")
	if id == "" {
		return "", "", false
	}
	if !found {
		return "", "", false
	}
	return id, "/v1/" + after, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
