package desk

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"kidtimer/daemon/internal/bank"
	"kidtimer/daemon/internal/household"
	"kidtimer/daemon/internal/netaddr"
	"kidtimer/daemon/internal/reverse"
	"kidtimer/fonts"
)

//go:embed kitchen.html
var kitchenPage []byte

type kitchenAsk struct {
	ID      string `json:"id"`
	Kid     string `json:"kid"`
	KidName string `json:"kid_name"`
	Seconds int    `json:"seconds"`
	Text    string `json:"text"`
}

type kitchenDoc struct {
	Asks []kitchenAsk `json:"asks"`
}

type kitchenDecideBody struct {
	Decision string `json:"decision"`
}

func kitchenRoute(path string) bool {
	switch path {
	case "/", "/font.ttf":
		return true
	}
	return strings.HasPrefix(path, "/v1/kitchen/")
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ip := netaddr.PeerIP(req.RemoteAddr)
		if kitchenRoute(req.URL.Path) {
			if !netaddr.IsHouseLAN(ip) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "house network only"})
				return
			}
			r.serveKitchen(w, req)
			return
		}
		if netaddr.Classify(ip) != netaddr.ClassLoopback {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "loopback only"})
			return
		}
		r.serveHTTP(w, req)
	})
}

func (r *Registry) serveKitchen(w http.ResponseWriter, req *http.Request) {
	switch {
	case req.URL.Path == "/" && req.Method == http.MethodGet:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(kitchenPage)
	case req.URL.Path == "/font.ttf" && req.Method == http.MethodGet:
		w.Header().Set("Content-Type", "font/ttf")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fonts.MonoRegular)
	case req.URL.Path == "/v1/kitchen/asks" && req.Method == http.MethodGet:
		r.handleKitchenAsks(w, req)
	default:
		kidID, askID, ok := kitchenDecidePath(req.URL.Path)
		if !ok || req.Method != http.MethodPost {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		r.handleKitchenDecide(w, req, kidID, askID)
	}
}

func kitchenDecidePath(path string) (kidID, askID string, ok bool) {
	const prefix = "/v1/kitchen/asks/"
	if !strings.HasPrefix(path, prefix) {
		return "", "", false
	}
	tail := strings.TrimPrefix(path, prefix)
	kidID, rest, found := strings.Cut(tail, "/")
	if !found || kidID == "" {
		return "", "", false
	}
	askID, verb, found := strings.Cut(rest, "/")
	if !found || askID == "" || verb != "decide" {
		return "", "", false
	}
	return kidID, askID, true
}

func (r *Registry) handleKitchenAsks(w http.ResponseWriter, req *http.Request) {
	_ = req
	kids, err := household.Load(r.path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if kids == nil {
		kids = []reverse.Record{}
	}
	writeJSON(w, http.StatusOK, kitchenDoc{Asks: kitchenAsks(r.Household(kids))})
}

func (r *Registry) handleKitchenDecide(w http.ResponseWriter, req *http.Request, kidID, askID string) {
	var body kitchenDecideBody
	if req.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
			return
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid body"})
				return
			}
		}
	}
	if body.Decision != "approve" && body.Decision != "deny" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "decision is required"})
		return
	}
	kids, err := household.Load(r.path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if kids == nil {
		kids = []reverse.Record{}
	}
	if !kitchenHasAsk(r.Household(kids), kidID, askID) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	payload, err := json.Marshal(map[string]string{"decision": body.Decision})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	res, err := r.Call(reverse.KidID(kidID), reverse.Op{
		Method:  http.MethodPost,
		Path:    "/v1/asks/" + askID + "/decide",
		Body:    payload,
		IdemKey: req.Header.Get("Idempotency-Key"),
	})
	if err != nil {
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

func kitchenAsks(hh reverse.Household) []kitchenAsk {
	out := make([]kitchenAsk, 0)
	for _, m := range hh.Kids {
		if m.ID == "" {
			continue
		}
		name := strings.TrimSpace(string(m.Name))
		if name == "" {
			name = "kid"
		}
		locked := statusLocked(m.Status)
		for _, a := range memberAsks(m.Asks) {
			if a.ID == "" {
				continue
			}
			if a.Status != "" && a.Status != bank.AskPending {
				continue
			}
			out = append(out, kitchenAsk{
				ID:      a.ID,
				Kid:     string(m.ID),
				KidName: name,
				Seconds: a.Seconds,
				Text:    askCardText(name, a.Seconds, locked),
			})
		}
	}
	return out
}

func kitchenHasAsk(hh reverse.Household, kidID, askID string) bool {
	for _, a := range kitchenAsks(hh) {
		if a.Kid == kidID && a.ID == askID {
			return true
		}
	}
	return false
}

func memberAsks(raw json.RawMessage) []bank.Ask {
	if len(raw) == 0 {
		return nil
	}
	var asks []bank.Ask
	if err := json.Unmarshal(raw, &asks); err == nil {
		return asks
	}
	var wrap struct {
		Asks []bank.Ask `json:"asks"`
	}
	if err := json.Unmarshal(raw, &wrap); err == nil {
		return wrap.Asks
	}
	return nil
}

func statusLocked(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var st struct {
		ParentLocked bool `json:"parent_locked"`
		CamelLocked  bool `json:"parentLocked"`
	}
	if json.Unmarshal(raw, &st) != nil {
		return false
	}
	return st.ParentLocked || st.CamelLocked
}

func askCardText(name string, seconds int, locked bool) string {
	m := int(float64(seconds)/60 + 0.5)
	if m < 1 {
		m = 10
	}
	if locked {
		return name + " asked to unlock for " + strconv.Itoa(m) + " more minutes"
	}
	return name + " asked for " + strconv.Itoa(m) + " more minutes"
}
