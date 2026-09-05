package dial

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kidtimer/daemon/internal/advertise"
	"kidtimer/daemon/internal/bank"
	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

const DefaultBankHTTP = "127.0.0.1:8742"

type Config struct {
	Home      string
	Name      reverse.KidName
	Bank      *bank.Bank
	Endpoints []reverse.Endpoint
	BankHTTP  string
}

type TestConfig = Config

func Run(ctx context.Context, cfg Config) error {
	if cfg.Home == "" {
		return fmt.Errorf("dial: home is required")
	}
	role, err := household.LoadRole(cfg.Home)
	if err != nil {
		return err
	}
	if role == reverse.RoleParent {
		return nil
	}
	stopBank, err := takeBank(ctx, &cfg)
	if err != nil {
		return err
	}
	defer stopBank()
	b := cfg.Bank
	if b == nil && cfg.BankHTTP == "" {
		return fmt.Errorf("dial: bank is required")
	}
	if cfg.Name == "" {
		host, _ := os.Hostname()
		cfg.Name = reverse.KidName(host)
	}
	id, err := advertise.MachineID(filepath.Join(cfg.Home, "machine-id"))
	if err != nil {
		return err
	}
	kidID := reverse.KidID(id)
	backoffs := []time.Duration{time.Second, 2 * time.Second, 5 * time.Second, 15 * time.Second, 15 * time.Second}
	step := 0
	for ctx.Err() == nil {
		eps := cfg.Endpoints
		if len(eps) == 0 {
			found, ferr := Find(ctx, cfg.Home)
			if ferr == nil {
				eps = found
			}
		}
		var last error
		for _, ep := range eps {
			if ctx.Err() != nil {
				return nil
			}
			h := &kidHandler{home: cfg.Home, bank: b, bankHTTP: cfg.BankHTTP, id: kidID, name: cfg.Name, parent: ep}
			hello, err := h.hello()
			if err != nil {
				last = err
				continue
			}
			last = reverse.OpenConn(ctx, ep, hello, h)
			if ctx.Err() != nil {
				return nil
			}
		}
		if last == nil && len(eps) == 0 {
			last = errors.New("no parent")
		}
		d := backoffs[step]
		if step < len(backoffs)-1 {
			step++
		}
		timer := time.NewTimer(d)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		_ = last
	}
	return nil
}

type kidHandler struct {
	home     string
	bank     *bank.Bank
	bankHTTP string
	secret   string
	tok      *bank.Token
	id       reverse.KidID
	name     reverse.KidName
	parent   reverse.Endpoint
}

func (h *kidHandler) hello() (reverse.Frame, error) {
	sess, ok, err := LoadSession(h.home)
	if err != nil {
		return reverse.Frame{}, err
	}
	if ok && sess.Ticket != "" && sess.ID != "" {
		h.id = sess.ID
		return reverse.Frame{Resume: &reverse.Resume{ID: sess.ID, Ticket: sess.Ticket}}, nil
	}
	return reverse.Frame{Offer: &reverse.Offer{ID: h.id, Name: h.name}}, nil
}

func (h *kidHandler) OnOffer(reverse.Offer) (*reverse.Accept, *reverse.Reject) {
	return nil, &reverse.Reject{Reason: reverse.RejectParentRole}
}
func (h *kidHandler) OnResume(reverse.Resume) (*reverse.ResumeOK, *reverse.Reject) {
	return nil, &reverse.Reject{Reason: reverse.RejectParentRole}
}

func (h *kidHandler) OnAccept(a reverse.Accept) error {
	if err := SaveSession(h.home, reverse.SessionFile{Parent: h.parent, Ticket: a.Ticket, ID: h.id}); err != nil {
		return err
	}
	return h.pairOnce()
}

func (h *kidHandler) OnResumeOK() error {
	return h.loadPair()
}

func (h *kidHandler) OnReject(r reverse.Reject) error {
	if r.Reason == reverse.RejectBadTicket {
		_ = os.Remove(sessionPath(h.home))
	}
	return fmt.Errorf("rejected: %d", r.Reason)
}

func (h *kidHandler) OnOp(op reverse.Op) reverse.OpResult {
	if h.bank == nil {
		if err := h.loadPair(); err != nil {
			return reverse.OpResult{Corr: op.Corr, Status: http.StatusUnauthorized, Body: json.RawMessage(`{"error":"unauthorized"}`)}
		}
		return Proxy(h.bankHTTP, h.secret, op)
	}
	if h.tok == nil {
		if err := h.loadPair(); err != nil {
			return reverse.OpResult{Corr: op.Corr, Status: http.StatusUnauthorized, Body: json.RawMessage(`{"error":"unauthorized"}`)}
		}
	}
	return Apply(h.bank, h.tok, op)
}

func (h *kidHandler) OnResult(reverse.OpResult) {}
func (h *kidHandler) OnDrop()                   {}

func (h *kidHandler) pairOnce() error {
	if h.bank == nil {
		return h.pairHTTP()
	}
	secret, tok, err := h.bank.Pair()
	if err != nil {
		if errors.Is(err, bank.ErrConflict) {
			return h.loadPair()
		}
		return err
	}
	if err := os.WriteFile(pairPath(h.home), []byte(secret+"\n"), 0o600); err != nil {
		return err
	}
	h.secret = secret
	h.tok = tok
	return nil
}

func (h *kidHandler) pairHTTP() error {
	if err := h.loadPair(); err == nil {
		return nil
	}
	resp, err := http.Post("http://"+h.bankHTTP+"/v1/pair", "application/json", strings.NewReader("{}"))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		return h.loadPair()
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pair: %s", resp.Status)
	}
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return err
	}
	if body.Token == "" {
		return fmt.Errorf("pair: empty token")
	}
	if err := os.MkdirAll(h.home, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(pairPath(h.home), []byte(body.Token+"\n"), 0o600); err != nil {
		return err
	}
	h.secret = body.Token
	return nil
}

func (h *kidHandler) loadPair() error {
	b, err := os.ReadFile(pairPath(h.home))
	if err != nil {
		return err
	}
	h.secret = strings.TrimSpace(string(b))
	if h.secret == "" {
		return fmt.Errorf("empty pair")
	}
	if h.bank == nil {
		return nil
	}
	tok, err := h.bank.LookupSecret(h.secret)
	if err != nil {
		return err
	}
	h.tok = tok
	return nil
}

func sessionPath(home string) string {
	return filepath.Join(home, "parent-session.json")
}

func pairPath(home string) string {
	return filepath.Join(home, "parent-pair")
}

func LoadSession(home string) (reverse.SessionFile, bool, error) {
	b, err := os.ReadFile(sessionPath(home))
	if err != nil {
		if os.IsNotExist(err) {
			return reverse.SessionFile{}, false, nil
		}
		return reverse.SessionFile{}, false, err
	}
	var s reverse.SessionFile
	if err := json.Unmarshal(b, &s); err != nil {
		return reverse.SessionFile{}, false, err
	}
	return s, s.Ticket != "", nil
}

func SaveSession(home string, s reverse.SessionFile) error {
	if err := os.MkdirAll(home, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := sessionPath(home)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func Find(ctx context.Context, home string) ([]reverse.Endpoint, error) {
	var out []reverse.Endpoint
	if sess, ok, err := LoadSession(home); err != nil {
		return nil, err
	} else if ok && sess.Parent != "" {
		out = append(out, sess.Parent)
	}
	out = append(out, loadHintEndpoints(home)...)
	found := make(chan advertise.Found, 8)
	mctx, cancel := context.WithTimeout(ctx, 400*time.Millisecond)
	defer cancel()
	go func() { _ = advertise.BrowseParent(mctx, found) }()
	for {
		select {
		case f := <-found:
			if ep := endpointFromURL(f.URL); ep != "" {
				out = append(out, ep)
			}
		case <-mctx.Done():
			if gw, err := GatewayEndpoint(8743); err == nil {
				out = append(out, gw)
			}
			return uniqEndpoints(out), nil
		}
	}
}

func endpointFromURL(u string) reverse.Endpoint {
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "https://")
	u = strings.TrimSuffix(u, "/")
	if u == "" {
		return ""
	}
	return reverse.Endpoint(u)
}

func uniqEndpoints(in []reverse.Endpoint) []reverse.Endpoint {
	seen := map[reverse.Endpoint]bool{}
	var out []reverse.Endpoint
	for _, e := range in {
		if e == "" || seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out
}

func GatewayEndpoint(port int) (reverse.Endpoint, error) {
	b, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return "", err
	}
	for i, line := range strings.Split(string(b), "\n") {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		if fields[1] != "00000000" {
			continue
		}
		raw, err := hex.DecodeString(fields[2])
		if err != nil || len(raw) != 4 {
			continue
		}
		ip := net.IPv4(raw[3], raw[2], raw[1], raw[0])
		if ip == nil || ip.IsUnspecified() {
			continue
		}
		return reverse.Endpoint(net.JoinHostPort(ip.String(), fmt.Sprintf("%d", port))), nil
	}
	return "", errors.New("no default gateway")
}

func PickupConfig(name reverse.KidName) (*config.Config, error) {
	if name == "" {
		name = "kid"
	}
	raw := fmt.Sprintf(`
kid_name = %q
timezone = "UTC"
enforcer = false
advertise = false
bedtime_lock = false
remote_lock = false
bedtime_start = "21:00"
bedtime_end = "07:00"

[groups.fun]
policy = "metered"
daily_seconds = 3600
`, name)
	return config.Parse([]byte(raw))
}
