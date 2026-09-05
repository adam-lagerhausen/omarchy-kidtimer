package desk

import (
	"context"
	"encoding/json"
	"net"
	"os/exec"
	"strconv"

	"kidtimer/daemon/internal/netaddr"
)

const kidBankPort = 8742

var tailscaleStatusJSON = func() ([]byte, error) {
	return exec.Command("tailscale", "status", "--json").Output()
}

type tsPeer struct {
	Online       bool     `json:"Online"`
	TailscaleIPs []string `json:"TailscaleIPs"`
}

type tsStatus struct {
	Self struct {
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Self"`
	Peer map[string]tsPeer `json:"Peer"`
}

func householdKidURLs(ctx context.Context) []string {
	_ = ctx
	raw, err := tailscaleStatusJSON()
	if err != nil || len(raw) == 0 {
		return nil
	}
	var st tsStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		return nil
	}
	self := map[string]bool{}
	for _, s := range st.Self.TailscaleIPs {
		self[s] = true
	}
	var out []string
	seen := map[string]bool{}
	for _, p := range st.Peer {
		if !p.Online {
			continue
		}
		for _, s := range p.TailscaleIPs {
			ip := net.ParseIP(s)
			if ip == nil || ip.To4() == nil || self[s] || !netaddr.IsHousehold(ip) {
				continue
			}
			u := "http://" + net.JoinHostPort(ip.String(), strconv.Itoa(kidBankPort))
			if seen[u] {
				continue
			}
			seen[u] = true
			out = append(out, u)
		}
	}
	return out
}
