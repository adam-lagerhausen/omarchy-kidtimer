package desk

import (
	"encoding/json"
	"net/http"
	"strings"

	"kidtimer/daemon/internal/reverse"
)

const (
	grantBlockLocked  = "locked"
	grantBlockBedtime = "bedtime"
	grantBlockBreak   = "on a break"
	grantBlockEmpty   = "no time left"
)

// GrantBlockReason matches the panel +10 / −10 rules. An empty status means
// the desk has not heard from that computer yet, so the grant is left to try.
func GrantBlockReason(status json.RawMessage, seconds int) string {
	raw := strings.TrimSpace(string(status))
	if raw == "" || raw == "null" {
		return ""
	}
	var st struct {
		ParentLocked  bool               `json:"parent_locked"`
		BedtimeActive bool               `json:"bedtime_active"`
		BedtimeHold   bool               `json:"bedtime_hold"`
		BreakSeconds  float64            `json:"break_seconds"`
		Groups        map[string]float64 `json:"groups"`
	}
	if err := json.Unmarshal(status, &st); err != nil {
		return ""
	}
	if st.ParentLocked {
		return grantBlockLocked
	}
	if st.BreakSeconds > 0 {
		return grantBlockBreak
	}
	if st.BedtimeActive && !st.BedtimeHold {
		return grantBlockBedtime
	}
	if seconds < 0 && int(st.Groups["fun"]) <= 0 {
		return grantBlockEmpty
	}
	return ""
}

func KnownGrantBlock(why string) bool {
	switch why {
	case grantBlockLocked, grantBlockBedtime, grantBlockBreak, grantBlockEmpty:
		return true
	default:
		return false
	}
}

func (r *Registry) refuseGrant(id reverse.KidID, body json.RawMessage) (string, bool) {
	res, err := r.Call(id, reverse.Op{Method: http.MethodGet, Path: "/v1/status"})
	if err != nil || res.Status != http.StatusOK {
		return "", false
	}
	var payload struct {
		Seconds int `json:"seconds"`
	}
	if len(body) > 0 && json.Unmarshal(body, &payload) != nil {
		return "", false
	}
	why := GrantBlockReason(res.Body, payload.Seconds)
	if why == "" {
		return "", false
	}
	return why, true
}
