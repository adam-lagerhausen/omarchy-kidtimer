package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/reverse"
)

const defaultDeskURL = "http://127.0.0.1:8741"

type Request struct {
	Target Target
	Action Action
}

type Target interface {
	target()
}

type Desk struct {
	URL  string
	Kid  string
	Home string
}

func (Desk) target() {}

type Direct struct {
	URL   string
	Token token
}

func (Direct) target() {}

type token struct{ s string }

func (token) String() string   { return "" }
func (token) GoString() string { return "token{}" }

func (t token) header() string {
	if t.s == "" {
		return ""
	}
	return "Bearer " + t.s
}

type Action interface {
	action()
}

type Lock struct{}

func (Lock) action() {}

type Unlock struct{}

func (Unlock) action() {}

type Grant struct {
	Group          string
	Seconds        int
	Reason         string
	IdempotencyKey string
}

func (Grant) action() {}

type Status struct{}

func (Status) action() {}

type Export struct {
	JSON bool
}

func (Export) action() {}

type Asks struct{}

func (Asks) action() {}

type Decide struct {
	ID       string
	Decision decision
}

func (Decide) action() {}

type decision string

const (
	decisionApprove decision = "approve"
	decisionDeny    decision = "deny"
)

func Parse(verb string, args []string) (Request, error) {
	fs := flag.NewFlagSet("kidtimer "+verb, flag.ContinueOnError)
	urlFlag := fs.String("url", "", "kid daemon URL")
	tokenFlag := fs.String("token", "", "bearer token")
	deskFlag := fs.String("desk", "", "parent desk URL")
	kidFlag := fs.String("kid", "", "kid name or id")
	homeFlag := (*string)(nil)
	pinHome := verb == "lock" || verb == "unlock"
	if pinHome {
		homeFlag = fs.String("home", "", "household dir")
	}
	var grant Grant
	var minutes int
	asJSON := false
	switch verb {
	case "grant":
		fs.StringVar(&grant.Group, "group", "", "group id")
		fs.IntVar(&grant.Seconds, "seconds", 900, "seconds to credit")
		fs.StringVar(&grant.Reason, "reason", "", "reason")
		fs.StringVar(&grant.IdempotencyKey, "idempotency-key", "", "Idempotency-Key")
		fs.IntVar(&minutes, "minutes", 0, "minutes to credit")
	case "export":
		fs.BoolVar(&asJSON, "json", false, "print today as JSON")
	case "lock", "unlock", "status", "asks", "decide":
	default:
		return Request{}, fmt.Errorf("usage: kidtimer lock|unlock|grant|status|asks|decide")
	}
	if err := fs.Parse(args); err != nil {
		return Request{}, err
	}
	kid := *kidFlag
	if verb == "export" {
		rest := fs.Args()
		if kid == "" && len(rest) == 1 {
			kid = rest[0]
			rest = nil
		}
		if len(rest) > 0 {
			return Request{}, fmt.Errorf("usage: kidtimer parent export [kid]")
		}
	}
	minutesSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "minutes" {
			minutesSet = true
		}
	})
	home := ""
	if homeFlag != nil {
		home = *homeFlag
	}
	target, err := selectTarget(*urlFlag, *tokenFlag, *deskFlag, kid, home, pinHome)
	if err != nil {
		return Request{}, err
	}
	var action Action
	switch verb {
	case "lock":
		action = Lock{}
	case "unlock":
		action = Unlock{}
	case "grant":
		if grant.Group == "" {
			grant.Group = "fun"
		}
		if minutesSet {
			grant.Seconds = minutes * 60
		}
		action = grant
	case "status":
		action = Status{}
	case "export":
		action = Export{JSON: asJSON}
	case "asks":
		action = Asks{}
	case "decide":
		action, err = parseDecide(fs.Args())
		if err != nil {
			return Request{}, err
		}
	}
	return Request{Target: target, Action: action}, nil
}

func selectTarget(urlFlag, tokenFlag, deskURL, kid, home string, pinHome bool) (Target, error) {
	daemonURL := strings.TrimSpace(urlFlag)
	if daemonURL == "" {
		daemonURL = strings.TrimSpace(os.Getenv("KIDTIMER_URL"))
	}
	tok := strings.TrimSpace(tokenFlag)
	if tok == "" {
		tok = strings.TrimSpace(os.Getenv("KIDTIMER_TOKEN"))
	}
	hasURL := daemonURL != ""
	hasTok := tok != ""
	if hasURL != hasTok {
		if hasURL {
			return nil, fmt.Errorf("lab needs a token (-token or KIDTIMER_TOKEN)")
		}
		return nil, fmt.Errorf("lab needs a URL (-url or KIDTIMER_URL)")
	}
	if hasURL && hasTok {
		if kid != "" || deskURL != "" {
			return nil, fmt.Errorf("do not mix -kid or -desk with a kid daemon URL")
		}
		return Direct{URL: strings.TrimRight(daemonURL, "/"), Token: token{s: tok}}, nil
	}
	if deskURL == "" {
		deskURL = defaultDeskURL
	}
	if err := requireLoopback(deskURL); err != nil {
		return nil, err
	}
	d := Desk{URL: strings.TrimRight(deskURL, "/"), Kid: kid}
	if pinHome {
		dir, err := shareDir(home)
		if err != nil {
			return nil, err
		}
		d.Home = dir
	}
	return d, nil
}

func parseDecide(rest []string) (Decide, error) {
	if len(rest) != 2 {
		return Decide{}, fmt.Errorf("usage: kidtimer decide <id> approve|deny")
	}
	switch rest[1] {
	case "approve":
		return Decide{ID: rest[0], Decision: decisionApprove}, nil
	case "deny":
		return Decide{ID: rest[0], Decision: decisionDeny}, nil
	default:
		return Decide{}, fmt.Errorf("usage: kidtimer decide <id> approve|deny")
	}
}

func requireLoopback(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("desk URL must be loopback")
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("desk URL must be loopback")
}

func Do(req Request) error {
	method, rest, body, idem, err := encodeAction(req.Action)
	if err != nil {
		return err
	}
	kidID := ""
	switch t := req.Target.(type) {
	case Desk:
		kids, err := fetchHousehold(t.URL)
		if err != nil {
			return err
		}
		kidID, err = resolveKid(kids, t.Kid)
		if err != nil {
			return err
		}
		switch req.Action.(type) {
		case Lock, Unlock:
			armed, err := deskLockArmed(t.Home, kids)
			if err != nil {
				return err
			}
			if !armed {
				return fmt.Errorf("no PIN set. Run kidtimer pin set")
			}
		}
	case Direct:
	default:
		return fmt.Errorf("unknown target")
	}
	addr, tok, err := route(req.Target, kidID, rest)
	if err != nil {
		return err
	}
	resp, err := send(method, addr, tok, body, idem)
	if err != nil {
		return err
	}
	if exp, ok := req.Action.(Export); ok {
		return writeExport(resp, exp.JSON)
	}
	return dump(resp, nil)
}

func encodeAction(a Action) (method, rest string, body []byte, idem string, err error) {
	switch a := a.(type) {
	case Lock:
		body, _ = json.Marshal(map[string]bool{"locked": true})
		return http.MethodPost, "/lock", body, "", nil
	case Unlock:
		body, _ = json.Marshal(map[string]bool{"locked": false})
		return http.MethodPost, "/lock", body, "", nil
	case Grant:
		if a.Group == "" {
			a.Group = "fun"
		}
		idem = a.IdempotencyKey
		if idem == "" {
			idem = fmt.Sprintf("cli-%d", time.Now().UnixNano())
		}
		body, _ = json.Marshal(map[string]any{
			"group": a.Group, "seconds": a.Seconds, "reason": a.Reason,
		})
		return http.MethodPost, "/grants", body, idem, nil
	case Status:
		return http.MethodGet, "/status", nil, "", nil
	case Export:
		return http.MethodGet, "/status", nil, "", nil
	case Asks:
		return http.MethodGet, "/asks", nil, "", nil
	case Decide:
		body, _ = json.Marshal(map[string]string{"decision": string(a.Decision)})
		return http.MethodPost, "/asks/" + a.ID + "/decide", body, "", nil
	default:
		return "", "", nil, "", fmt.Errorf("unknown action")
	}
}

func route(t Target, kidID, rest string) (string, token, error) {
	switch t := t.(type) {
	case Desk:
		return strings.TrimRight(t.URL, "/") + "/v1/kids/" + kidID + rest, token{}, nil
	case Direct:
		return strings.TrimRight(t.URL, "/") + "/v1" + rest, t.Token, nil
	default:
		return "", token{}, fmt.Errorf("unknown target")
	}
}

func fetchHousehold(deskURL string) ([]reverse.Member, error) {
	resp, err := send(http.MethodGet, strings.TrimRight(deskURL, "/")+"/v1/household", token{}, nil, "")
	if err != nil {
		return nil, fmt.Errorf("parent desk is not running on %s", deskURL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("parent desk is not running on %s", deskURL)
	}
	var hh reverse.Household
	if err := json.NewDecoder(resp.Body).Decode(&hh); err != nil {
		return nil, fmt.Errorf("parent desk is not running on %s", deskURL)
	}
	return hh.Kids, nil
}

func resolveKid(kids []reverse.Member, hint string) (string, error) {
	hint = strings.TrimSpace(hint)
	if hint == "" {
		var live []reverse.Member
		for _, k := range kids {
			if k.Live && k.ID != "" {
				live = append(live, k)
			}
		}
		if len(live) == 1 {
			return string(live[0].ID), nil
		}
		return "", fmt.Errorf("which kid? pass -kid")
	}
	for _, k := range kids {
		if string(k.ID) == hint {
			if !k.Live {
				return "", notConnected(k, hint)
			}
			return string(k.ID), nil
		}
	}
	var named []reverse.Member
	for _, k := range kids {
		if strings.EqualFold(string(k.Name), hint) {
			named = append(named, k)
		}
	}
	if len(named) == 1 {
		if !named[0].Live {
			return "", notConnected(named[0], hint)
		}
		return string(named[0].ID), nil
	}
	if len(named) > 1 {
		return "", fmt.Errorf("which kid? pass -kid")
	}
	return "", fmt.Errorf("unknown name")
}

func notConnected(k reverse.Member, hint string) error {
	name := strings.TrimSpace(string(k.Name))
	if name == "" {
		name = hint
	}
	return fmt.Errorf("%s is not connected", name)
}

func deskLockArmed(home string, kids []reverse.Member) (bool, error) {
	ok, err := pinSet(home)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	for _, k := range kids {
		if k.Live && kidPinSet(k.Status) {
			return true, nil
		}
	}
	return false, nil
}

func pinSet(home string) (bool, error) {
	_, ok, err := household.ReadPin(home)
	return ok, err
}

func kidPinSet(status json.RawMessage) bool {
	var st struct {
		ParentPinSet bool `json:"parent_pin_set"`
	}
	if err := json.Unmarshal(status, &st); err != nil {
		return false
	}
	return st.ParentPinSet
}

func send(method, addr string, tok token, body []byte, idem string) (*http.Response, error) {
	var rdr io.Reader
	if len(body) > 0 {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, addr, rdr)
	if err != nil {
		return nil, err
	}
	if h := tok.header(); h != "" {
		req.Header.Set("Authorization", h)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	return http.DefaultClient.Do(req)
}
