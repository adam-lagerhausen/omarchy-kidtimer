package dial

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kidtimer/daemon/internal/bank"
	"kidtimer/daemon/internal/look"
	"kidtimer/daemon/internal/reverse"
)

func loadHintEndpoints(home string) []reverse.Endpoint {
	b, err := os.ReadFile(filepath.Join(home, "parent-endpoints"))
	if err != nil {
		return nil
	}
	var out []reverse.Endpoint
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, reverse.Endpoint(line))
	}
	return out
}

func Proxy(addr, secret string, op reverse.Op) reverse.OpResult {
	if addr == "" || secret == "" {
		return fail(op, http.StatusUnauthorized, "unauthorized")
	}
	req, err := http.NewRequest(op.Method, "http://"+addr+op.Path, bytes.NewReader(nonzero(op.Body)))
	if err != nil {
		return fail(op, http.StatusBadGateway, "bad request")
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if op.IdemKey != "" {
		req.Header.Set("Idempotency-Key", op.IdemKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fail(op, http.StatusBadGateway, "offline")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		body = []byte("{}")
	}
	return reverse.OpResult{Corr: op.Corr, Status: resp.StatusCode, Body: body}
}

func Apply(b *bank.Bank, tok *bank.Token, op reverse.Op) reverse.OpResult {
	if b == nil || tok == nil {
		return fail(op, http.StatusUnauthorized, "unauthorized")
	}
	path := op.Path
	switch {
	case op.Method == http.MethodGet && path == "/v1/status":
		st, err := b.Status(tok)
		if err != nil {
			return failErr(op, err)
		}
		return ok(op, statusJSON(st))
	case op.Method == http.MethodPost && path == "/v1/grants":
		var body struct {
			Group   string `json:"group"`
			Seconds int    `json:"seconds"`
			Reason  string `json:"reason"`
		}
		if err := json.Unmarshal(nonzero(op.Body), &body); err != nil {
			return fail(op, http.StatusBadRequest, "invalid json")
		}
		if body.Group == "" {
			body.Group = "fun"
		}
		g, err := b.Grant(tok, body.Group, body.Seconds, body.Reason, op.IdemKey)
		if err != nil {
			return failErr(op, err)
		}
		return ok(op, map[string]any{
			"group":     g.Group,
			"seconds":   g.Seconds,
			"source":    g.Source,
			"reason":    g.Reason,
			"remaining": g.Remaining,
			"replay":    g.Replay,
		})
	case op.Method == http.MethodGet && path == "/v1/asks":
		asks, err := b.PendingAsks(tok)
		if err != nil {
			return failErr(op, err)
		}
		if asks == nil {
			asks = []bank.Ask{}
		}
		return ok(op, map[string]any{"asks": asks})
	case op.Method == http.MethodPost && strings.HasPrefix(path, "/v1/asks/") && strings.HasSuffix(path, "/decide"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/v1/asks/"), "/decide")
		var body struct {
			Decision string `json:"decision"`
		}
		if err := json.Unmarshal(nonzero(op.Body), &body); err != nil {
			return fail(op, http.StatusBadRequest, "invalid json")
		}
		g, ask, err := b.Decide(tok, id, body.Decision)
		if err != nil {
			return failErr(op, err)
		}
		out := map[string]any{
			"id":      ask.ID,
			"group":   ask.Group,
			"seconds": ask.Seconds,
			"reason":  ask.Reason,
			"status":  ask.Status,
		}
		if g != nil {
			out["grant"] = map[string]any{"group": g.Group, "seconds": g.Seconds, "remaining": g.Remaining}
		}
		return ok(op, out)
	case op.Method == http.MethodPost && path == "/v1/lock":
		var body struct {
			Locked *bool `json:"locked"`
		}
		if err := json.Unmarshal(nonzero(op.Body), &body); err != nil || body.Locked == nil {
			return fail(op, http.StatusBadRequest, "locked is required")
		}
		if err := b.SetParentLock(tok, *body.Locked); err != nil {
			return failErr(op, err)
		}
		return ok(op, map[string]any{"locked": *body.Locked})
	case op.Method == http.MethodPut && path == "/v1/parent-pin":
		var body struct {
			Pin  string `json:"pin"`
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal(nonzero(op.Body), &body); err != nil {
			return fail(op, http.StatusBadRequest, "invalid json")
		}
		if err := b.SetParentPIN(tok, body.Pin, body.Hash); err != nil {
			return failErr(op, err)
		}
		return ok(op, map[string]any{"parent_pin_set": true})
	case op.Method == http.MethodGet && path == "/v1/look":
		doc, err := b.Look(tok)
		if err != nil {
			return failErr(op, err)
		}
		return ok(op, lookJSON(doc))
	case op.Method == http.MethodPut && path == "/v1/look":
		var doc look.Document
		if err := json.Unmarshal(nonzero(op.Body), &doc); err != nil {
			return fail(op, http.StatusBadRequest, "invalid json")
		}
		if err := b.PutLook(tok, doc); err != nil {
			return failErr(op, err)
		}
		out, err := b.Look(tok)
		if err != nil {
			return failErr(op, err)
		}
		return ok(op, lookJSON(out))
	case op.Method == http.MethodPatch && path == "/v1/policy":
		var body struct {
			BedtimeStart *string `json:"bedtime_start"`
			BedtimeEnd   *string `json:"bedtime_end"`
			BedtimeLock  *bool   `json:"bedtime_lock"`
		}
		if err := json.Unmarshal(nonzero(op.Body), &body); err != nil {
			return fail(op, http.StatusBadRequest, "invalid json")
		}
		start, end := "", ""
		if body.BedtimeStart != nil {
			start = *body.BedtimeStart
		}
		if body.BedtimeEnd != nil {
			end = *body.BedtimeEnd
		}
		if err := b.SetBedtime(tok, start, end, body.BedtimeLock); err != nil {
			return failErr(op, err)
		}
		st, err := b.Status(tok)
		if err != nil {
			return failErr(op, err)
		}
		return ok(op, statusJSON(st))
	default:
		return fail(op, http.StatusNotFound, "not found")
	}
}

func nonzero(b json.RawMessage) []byte {
	if len(b) == 0 {
		return []byte("{}")
	}
	return b
}

func statusJSON(st *bank.Status) map[string]any {
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
		"parent_locked":     st.ParentLocked,
		"remote_lock":       st.RemoteLock,
		"parent_pin_set":    st.ParentPinSet,
		"overlay":           st.Overlay,
		"bedtime_hold":      st.BedtimeHold,
		"bedtime_start":     st.BedtimeStart,
		"bedtime_end":       st.BedtimeEnd,
		"piles":             piles,
		"spent":             st.Spent,
		"today":             todaySpans(st),
	}
	if st.Mode != "" {
		out["mode"] = st.Mode
	}
	if st.BedtimeIn != nil {
		out["bedtime_in"] = *st.BedtimeIn
	}
	if !st.OverrideUntil.IsZero() {
		out["override_until"] = st.OverrideUntil.UTC().Format(time.RFC3339)
	}
	return out
}

func todaySpans(st *bank.Status) []bank.TodaySpan {
	if st == nil || st.Today == nil {
		return []bank.TodaySpan{}
	}
	return st.Today
}

func lookJSON(doc look.Document) map[string]any {
	return map[string]any{
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
	}
}

func ok(op reverse.Op, v any) reverse.OpResult {
	raw, _ := json.Marshal(v)
	return reverse.OpResult{Corr: op.Corr, Status: http.StatusOK, Body: raw}
}

func fail(op reverse.Op, status int, msg string) reverse.OpResult {
	raw, _ := json.Marshal(map[string]string{"error": msg})
	return reverse.OpResult{Corr: op.Corr, Status: status, Body: raw}
}

func failErr(op reverse.Op, err error) reverse.OpResult {
	switch {
	case errors.Is(err, bank.ErrUnauthorized):
		return fail(op, http.StatusUnauthorized, "unauthorized")
	case errors.Is(err, bank.ErrForbidden):
		return fail(op, http.StatusForbidden, err.Error())
	case errors.Is(err, bank.ErrConflict):
		return fail(op, http.StatusConflict, err.Error())
	case errors.Is(err, bank.ErrNotFound):
		return fail(op, http.StatusNotFound, err.Error())
	case errors.Is(err, bank.ErrInvalid):
		return fail(op, http.StatusBadRequest, err.Error())
	default:
		return fail(op, http.StatusInternalServerError, "internal")
	}
}
