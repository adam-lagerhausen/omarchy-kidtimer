package desk

import (
	"encoding/json"
	"testing"
)

func TestGrantBlockReasonMatchesPanel(t *testing.T) {
	cases := []struct {
		name    string
		status  string
		seconds int
		want    string
	}{
		{name: "open", status: `{"parent_locked":false,"groups":{"fun":600}}`, seconds: 600, want: ""},
		{name: "locked plus", status: `{"parent_locked":true,"groups":{"fun":600}}`, seconds: 600, want: "locked"},
		{name: "locked minus", status: `{"parent_locked":true,"groups":{"fun":600}}`, seconds: -600, want: "locked"},
		{name: "break", status: `{"break_seconds":12,"groups":{"fun":600}}`, seconds: 600, want: "on a break"},
		{name: "bedtime", status: `{"bedtime_active":true,"groups":{"fun":600}}`, seconds: 600, want: "bedtime"},
		{name: "stay up", status: `{"bedtime_active":true,"bedtime_hold":true,"groups":{"fun":600}}`, seconds: 600, want: ""},
		{name: "empty minus", status: `{"groups":{"fun":0}}`, seconds: -600, want: "no time left"},
		{name: "last minute minus", status: `{"groups":{"fun":30}}`, seconds: -600, want: ""},
		{name: "empty plus", status: `{"groups":{"fun":0}}`, seconds: 600, want: ""},
		{name: "unheard", status: "", seconds: 600, want: ""},
		{name: "null", status: "null", seconds: -600, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := GrantBlockReason(json.RawMessage(tc.status), tc.seconds)
			if got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestKnownGrantBlock(t *testing.T) {
	if !KnownGrantBlock("locked") || !KnownGrantBlock("on a break") || KnownGrantBlock("conflict") {
		t.Fatal("known reasons")
	}
}
