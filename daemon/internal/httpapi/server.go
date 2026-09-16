package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kidtimer/daemon/internal/bank"
	"kidtimer/daemon/internal/look"
	"kidtimer/daemon/internal/netaddr"
)

type Server struct {
	Bank      *bank.Bank
	Resume    func(group string)
	SearchEnv func() ([]look.InstalledApp, look.Focus, func(string) bool)
	MachineID string
	recentWin map[string]string
}

func New(b *bank.Bank) *Server {
	return &Server{Bank: b}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/grants", s.handleGrant)
	mux.HandleFunc("GET /v1/status", s.handleStatus)
	mux.HandleFunc("POST /v1/asks", s.handleCreateAsk)
	mux.HandleFunc("GET /v1/asks", s.handleListAsks)
	mux.HandleFunc("POST /v1/asks/{id}/decide", s.handleDecide)
	mux.HandleFunc("POST /v1/pair", s.handlePair)
	mux.HandleFunc("POST /v1/reclaim", s.handleReclaim)
	mux.HandleFunc("POST /v1/tokens", s.handleMint)
	mux.HandleFunc("POST /v1/lock", s.handleLock)
	mux.HandleFunc("PUT /v1/parent-pin", s.handlePutPin)
	mux.HandleFunc("POST /v1/pin/approve", s.handlePinApprove)
	mux.HandleFunc("POST /v1/pin/grant", s.handlePinGrant)
	mux.HandleFunc("POST /v1/mode", s.handleMode)
	mux.HandleFunc("PATCH /v1/policy", s.handlePolicy)
	mux.HandleFunc("GET /v1/look", s.handleGetLook)
	mux.HandleFunc("PUT /v1/look", s.handlePutLook)
	mux.HandleFunc("GET /v1/search", s.handleSearch)
	return mux
}

type grantBody struct {
	Group   string `json:"group"`
	Seconds int    `json:"seconds"`
	Reason  string `json:"reason"`
}

type askBody struct {
	Group   string `json:"group"`
	Seconds int    `json:"seconds"`
	Reason  string `json:"reason"`
}

type decideBody struct {
	Decision string `json:"decision"`
}

type mintBody struct {
	Name               string   `json:"name"`
	Kind               string   `json:"kind"`
	Groups             []string `json:"groups"`
	MaxSecondsPerGrant int      `json:"max_seconds_per_grant"`
	MaxSecondsPerDay   int      `json:"max_seconds_per_day"`
}

type lockBody struct {
	Locked *bool `json:"locked"`
}

type modeBody struct {
	ID      string  `json:"id"`
	Until   *string `json:"until"`
	Minutes *int    `json:"minutes"`
}

type policyBody struct {
	BedtimeStart *string                   `json:"bedtime_start"`
	BedtimeEnd   *string                   `json:"bedtime_end"`
	BedtimeLock  *bool                     `json:"bedtime_lock"`
	Hour12       *bool                     `json:"hour12"`
	Modes        map[string]map[string]int `json:"modes"`
}

func (s *Server) handleGrant(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	var body grantBody
	if !decode(w, r, &body) {
		return
	}
	idem := r.Header.Get("Idempotency-Key")
	g, err := s.Bank.Grant(tok, body.Group, body.Seconds, body.Reason, idem)
	if err != nil {
		writeErr(w, err)
		return
	}
	if s.Resume != nil && !g.Replay && g.Seconds > 0 {
		s.Resume(g.Group)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"group":     g.Group,
		"seconds":   g.Seconds,
		"source":    g.Source,
		"reason":    g.Reason,
		"remaining": g.Remaining,
		"replay":    g.Replay,
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	s.writeStatus(w, tok)
}

func (s *Server) handleCreateAsk(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	var body askBody
	if !decode(w, r, &body) {
		return
	}
	ask, err := s.Bank.CreateAsk(tok, body.Group, body.Seconds, body.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":      ask.ID,
		"group":   ask.Group,
		"seconds": ask.Seconds,
		"reason":  ask.Reason,
		"status":  ask.Status,
	})
}

func (s *Server) handleListAsks(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	asks, err := s.Bank.PendingAsks(tok)
	if err != nil {
		writeErr(w, err)
		return
	}
	if asks == nil {
		asks = []bank.Ask{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"asks": asks})
}

func (s *Server) handleDecide(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	var body decideBody
	if !decode(w, r, &body) {
		return
	}
	g, ask, err := s.Bank.Decide(tok, r.PathValue("id"), body.Decision)
	if err != nil {
		if errors.Is(err, bank.ErrConflict) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "already decided"})
			return
		}
		writeErr(w, err)
		return
	}
	if g != nil && s.Resume != nil {
		s.Resume(g.Group)
	}
	out := map[string]any{
		"id":      ask.ID,
		"group":   ask.Group,
		"seconds": ask.Seconds,
		"reason":  ask.Reason,
		"status":  ask.Status,
	}
	if g != nil {
		out["grant"] = map[string]any{
			"group":     g.Group,
			"seconds":   g.Seconds,
			"source":    g.Source,
			"remaining": g.Remaining,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	s.writePair(w, r, s.Bank.Pair)
}

func (s *Server) handleReclaim(w http.ResponseWriter, r *http.Request) {
	s.writePair(w, r, s.Bank.Reclaim)
}

func (s *Server) writePair(w http.ResponseWriter, r *http.Request, fn func() (string, *bank.Token, error)) {
	ip := netaddr.PeerIP(r.RemoteAddr)
	if ip == nil || !ip.IsLoopback() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	secret, tok, err := fn()
	if err != nil {
		writeErr(w, err)
		return
	}
	id := s.MachineID
	if id == "" {
		id = tok.ID
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":  s.Bank.Config().KidName,
		"id":    id,
		"token": secret,
	})
}

func (s *Server) handleMint(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	var body mintBody
	if !decode(w, r, &body) {
		return
	}
	secret, minted, err := s.Bank.Mint(tok, bank.MintSpec{
		Name:               body.Name,
		Kind:               bank.Kind(body.Kind),
		Groups:             body.Groups,
		MaxSecondsPerGrant: body.MaxSecondsPerGrant,
		MaxSecondsPerDay:   body.MaxSecondsPerDay,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":                    minted.ID,
		"name":                  minted.Name,
		"kind":                  minted.Kind,
		"groups":                minted.Groups,
		"max_seconds_per_grant": minted.MaxSecondsPerGrant,
		"max_seconds_per_day":   minted.MaxSecondsPerDay,
		"secret":                secret,
	})
}

func (s *Server) handleLock(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	var body lockBody
	if !decode(w, r, &body) {
		return
	}
	if body.Locked == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "locked is required"})
		return
	}
	if err := s.Bank.SetParentLock(tok, *body.Locked); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"locked": *body.Locked})
}

type pinSetBody struct {
	Pin  string `json:"pin"`
	Hash string `json:"hash"`
}

type pinApproveBody struct {
	Pin   string `json:"pin"`
	AskID string `json:"ask_id"`
}

type pinGrantBody struct {
	Pin     string `json:"pin"`
	Seconds int    `json:"seconds"`
}

func (s *Server) handlePutPin(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	var body pinSetBody
	if !decode(w, r, &body) {
		return
	}
	if err := s.Bank.SetParentPIN(tok, body.Pin, body.Hash); err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"parent_pin_set": true})
}

func (s *Server) handlePinApprove(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	var body pinApproveBody
	if !decode(w, r, &body) {
		return
	}
	g, ask, err := s.Bank.PinApprove(tok, body.Pin, body.AskID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if g != nil && s.Resume != nil {
		s.Resume(g.Group)
	}
	out := map[string]any{
		"id":      ask.ID,
		"group":   ask.Group,
		"seconds": ask.Seconds,
		"reason":  ask.Reason,
		"status":  ask.Status,
	}
	if g != nil {
		out["grant"] = map[string]any{
			"group":     g.Group,
			"seconds":   g.Seconds,
			"source":    g.Source,
			"remaining": g.Remaining,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handlePinGrant(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	var body pinGrantBody
	if !decode(w, r, &body) {
		return
	}
	g, err := s.Bank.PinGrant(tok, body.Pin, body.Seconds)
	if err != nil {
		writeErr(w, err)
		return
	}
	if s.Resume != nil && g.Seconds > 0 {
		s.Resume(g.Group)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"group":     g.Group,
		"seconds":   g.Seconds,
		"source":    g.Source,
		"reason":    g.Reason,
		"remaining": g.Remaining,
	})
}

func (s *Server) handleMode(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	var body modeBody
	if !decode(w, r, &body) {
		return
	}
	spec, err := parseModeApply(body)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := s.Bank.ApplyModeSpec(tok, spec); err != nil {
		writeErr(w, err)
		return
	}
	id := spec.ID
	if spec.Kind == bank.UntilResume {
		st, stErr := s.Bank.Status(tok)
		if stErr != nil {
			writeErr(w, stErr)
			return
		}
		id = st.Mode
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

func parseModeApply(body modeBody) (bank.ModeApply, error) {
	until := ""
	if body.Until != nil {
		until = *body.Until
	}
	if until == "end" {
		return bank.ModeApply{Kind: bank.UntilResume}, nil
	}
	if body.Minutes != nil {
		if until != "" && until != "next" {
			return bank.ModeApply{}, fmt.Errorf("%w: minutes cannot combine with until %q", bank.ErrInvalid, until)
		}
		if body.ID == "" {
			return bank.ModeApply{}, fmt.Errorf("%w: id is required", bank.ErrInvalid)
		}
		return bank.ModeApply{ID: body.ID, Kind: bank.UntilFor, Minutes: *body.Minutes}, nil
	}
	if until == "sticky" {
		id := body.ID
		if id == "" {
			id = look.FreetimeID
		}
		return bank.ModeApply{ID: id, Kind: bank.UntilSticky}, nil
	}
	if until == "next" {
		if body.ID == "" {
			return bank.ModeApply{}, fmt.Errorf("%w: id is required", bank.ErrInvalid)
		}
		return bank.ModeApply{ID: body.ID, Kind: bank.UntilNext}, nil
	}
	if until != "" {
		return bank.ModeApply{}, fmt.Errorf("%w: until must be next, sticky, or end", bank.ErrInvalid)
	}
	if body.ID == "" {
		return bank.ModeApply{}, fmt.Errorf("%w: id is required", bank.ErrInvalid)
	}
	return bank.ModeApply{ID: body.ID, Kind: bank.UntilDefault}, nil
}

func (s *Server) handlePolicy(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	var body policyBody
	if !decode(w, r, &body) {
		return
	}
	start, end := "", ""
	if body.BedtimeStart != nil {
		start = *body.BedtimeStart
	}
	if body.BedtimeEnd != nil {
		end = *body.BedtimeEnd
	}
	if start != "" || end != "" || body.BedtimeLock != nil {
		if err := s.Bank.SetBedtime(tok, start, end, body.BedtimeLock); err != nil {
			writeErr(w, err)
			return
		}
	}
	if body.Hour12 != nil {
		if err := s.Bank.SetHour12(tok, *body.Hour12); err != nil {
			writeErr(w, err)
			return
		}
	}
	for modeID, groups := range body.Modes {
		for group, seconds := range groups {
			if err := s.Bank.SetModeMinutes(tok, modeID, group, seconds); err != nil {
				writeErr(w, err)
				return
			}
		}
	}
	s.writeStatus(w, tok)
}

func (s *Server) handleGetLook(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	doc, err := s.Bank.Look(tok)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeLook(w, doc)
}

func (s *Server) handlePutLook(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	stored, err := s.Bank.Look(tok)
	if err != nil {
		writeErr(w, err)
		return
	}
	var doc look.Document
	if !decode(w, r, &doc) {
		return
	}
	installed, _, _ := s.env()
	filled, err := look.ApplyIncoming(stored, doc, look.Fill{Installed: installed, WinClass: s.recentWin})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.Bank.PutLook(tok, filled); err != nil {
		writeErr(w, err)
		return
	}
	out, err := s.Bank.Look(tok)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeLook(w, out)
}

func writeLook(w http.ResponseWriter, doc look.Document) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version":         doc.Version,
		"piles":           doc.Piles,
		"things":          doc.Things,
		"apps":            doc.Apps,
		"pile_hours":      doc.PileHours,
		"fun_hours":       doc.FunHours,
		"modes":           doc.Modes,
		"schedule":        doc.Schedule,
		"bedtime":         doc.Bedtime,
		"sticky_freetime": doc.StickyFreetime,
	})
}

func (s *Server) env() ([]look.InstalledApp, look.Focus, func(string) bool) {
	if s.SearchEnv != nil {
		return s.SearchEnv()
	}
	return nil, look.Focus{}, nil
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	tok, ok := s.token(w, r)
	if !ok {
		return
	}
	doc, err := s.Bank.Look(tok)
	if err != nil {
		writeErr(w, err)
		return
	}
	q := r.URL.Query().Get("q")
	list := look.ListID(r.URL.Query().Get("list"))
	if list != look.ListFun && list != look.ListSchool {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "list must be fun or school"})
		return
	}
	installed, focus, alwaysOn := s.env()
	res := look.Search(q, list, installed, focus, alwaysOn, doc.Apps)
	if s.recentWin == nil {
		s.recentWin = map[string]string{}
	}
	if res.Focus != nil && strings.HasPrefix(res.Focus.ID, "win:") && focus.Class != "" {
		s.recentWin[res.Focus.ID] = focus.Class
	}
	writeJSON(w, http.StatusOK, searchJSON(res))
}

func searchJSON(res look.SearchResult) map[string]any {
	out := map[string]any{
		"q":      res.Q,
		"hits":   []any{},
		"notice": nil,
	}
	if res.Focus != nil {
		out["focus"] = hitJSON(*res.Focus)
	}
	hits := make([]any, 0, len(res.Hits))
	for _, h := range res.Hits {
		hits = append(hits, hitJSON(h))
	}
	out["hits"] = hits
	return out
}

func hitJSON(h look.Hit) map[string]any {
	m := map[string]any{"id": h.ID, "name": h.Name, "kind": h.Kind, "source": h.Source}
	if h.Focused {
		m["focused"] = true
	}
	return m
}

func (s *Server) writeStatus(w http.ResponseWriter, tok *bank.Token) {
	st, err := s.Bank.Status(tok)
	if err != nil {
		writeErr(w, err)
		return
	}
	groups := map[string]int{}
	for _, g := range st.Groups {
		groups[g.ID] = g.Remaining
	}
	var focused any
	if st.FocusedGroup != "" {
		focused = st.FocusedGroup
	}
	var focusedApp any
	if st.FocusedApp != "" {
		focusedApp = st.FocusedApp
	}
	var mode any
	if st.Mode != "" {
		mode = st.Mode
	}
	piles := st.Piles
	if piles == nil {
		piles = []look.Pile{}
	}
	out := map[string]any{
		"kid_name":          st.KidName,
		"groups":            groups,
		"path_remaining":    st.PathRemaining,
		"bedtime_active":    st.BedtimeActive,
		"focused_group":     focused,
		"focused_app":       focusedApp,
		"look_version":      st.LookVersion,
		"pending_ask_count": st.PendingAskCount,
		"mode":              mode,
		"parent_locked":     st.ParentLocked,
		"remote_lock":       st.RemoteLock,
		"parent_pin_set":    st.ParentPinSet,
		"overlay":           st.Overlay,
		"bedtime_hold":      st.BedtimeHold,
		"bedtime_start":     st.BedtimeStart,
		"bedtime_end":       st.BedtimeEnd,
		"hour12":            st.Hour12,
		"modes":             st.Modes,
		"piles":             piles,
		"spent":             st.Spent,
		"today":             todaySpans(st),
	}
	if st.BedtimeIn != nil {
		out["bedtime_in"] = *st.BedtimeIn
	}
	if !st.OverrideUntil.IsZero() {
		out["override_until"] = st.OverrideUntil.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

func todaySpans(st *bank.Status) []bank.TodaySpan {
	if st == nil || st.Today == nil {
		return []bank.TodaySpan{}
	}
	return st.Today
}

func bearerSecret(header string) (string, bool) {
	const prefix = "bearer "
	if len(header) < len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	secret := strings.TrimSpace(header[len(prefix):])
	return secret, secret != ""
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) (*bank.Token, bool) {
	secret, ok := bearerSecret(r.Header.Get("Authorization"))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil, false
	}
	tok, err := s.Bank.LookupSecret(secret)
	if err != nil {
		writeErr(w, err)
		return nil, false
	}
	return tok, true
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return false
	}
	return true
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, bank.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	case errors.Is(err, bank.ErrForbidden):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": err.Error()})
	case errors.Is(err, bank.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	case errors.Is(err, bank.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, bank.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
