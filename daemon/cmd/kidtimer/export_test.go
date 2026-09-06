package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFormatTodayTable(t *testing.T) {
	got := formatTodayTable([]exportSpan{
		{Kind: "on", Start: 16*60 + 59, Dur: 16, Label: "foot"},
		{Kind: "on", Start: 17*60 + 14, Dur: 60, Label: "foot"},
		{Kind: "on", Start: 8 * 60, Dur: 5, Label: ""},
	})
	want := "16:59  foot  16m\n17:14  foot  1h\n08:00  on  5m\n"
	if got != want {
		t.Fatalf("table %q want %q", got, want)
	}
	unix := time.Date(2026, 8, 26, 16, 59, 0, 0, time.Local).Unix()
	got = formatTodayTable([]exportSpan{{Kind: "on", Start: 0, StartUnix: unix, Dur: 16, Label: "foot"}})
	if got != "16:59  foot  16m\n" {
		t.Fatalf("unix table %q", got)
	}
}

func TestExportTodayJSON(t *testing.T) {
	raw := []byte(`{"groups":{"fun":100},"today":[{"kind":"on","start":1019,"dur":16,"label":"foot"}]}`)
	got, err := exportTodayJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"kind":"on","start":1019,"dur":16,"label":"foot"}]` + "\n"
	if string(got) != want {
		t.Fatalf("json %q want %q", got, want)
	}
	empty, err := exportTodayJSON([]byte(`{"groups":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) != "[]\n" {
		t.Fatalf("missing today %q", empty)
	}
}

func TestParseExportPositionalKid(t *testing.T) {
	clearLabEnv(t)
	req, err := Parse("export", []string{"Ada"})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := req.Target.(Desk)
	if !ok {
		t.Fatalf("target %T", req.Target)
	}
	if d.Kid != "Ada" {
		t.Fatalf("kid %s", d.Kid)
	}
	if _, ok := req.Action.(Export); !ok {
		t.Fatalf("action %T", req.Action)
	}
}

func TestParseExportJSON(t *testing.T) {
	clearLabEnv(t)
	req, err := Parse("export", []string{"-json", "-kid", "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	exp, ok := req.Action.(Export)
	if !ok || !exp.JSON {
		t.Fatalf("json %+v", req.Action)
	}
}

func TestParentExportDeskStatus(t *testing.T) {
	clearLabEnv(t)
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization %q on %s", r.Header.Get("Authorization"), r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/household":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"kids":[{"id":"kid-1","name":"Ada","live":true}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kids/kid-1/status":
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"today":[{"kind":"on","start":1019,"dur":16,"label":"foot"},{"kind":"on","start":1034,"dur":1,"label":"foot"}]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	out, err := captureStdout(t, func() error {
		return runParent([]string{"export", "-desk", srv.URL, "-kid", "Ada"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/kids/kid-1/status" {
		t.Fatalf("path %s", gotPath)
	}
	if out != "16:59  foot  16m\n17:14  foot  1m\n" {
		t.Fatalf("out %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "parent.lock")); !os.IsNotExist(err) {
		t.Fatalf("export must not lock the desk: %v", err)
	}
}

func TestParentExportJSON(t *testing.T) {
	clearLabEnv(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/household":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"kids":[{"id":"kid-1","name":"Ada","live":true}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/kids/kid-1/status":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"today":[{"kind":"on","start":1019,"dur":16,"label":"foot"}]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	out, err := captureStdout(t, func() error {
		return runParent([]string{"export", "--json", "-desk", srv.URL, "Ada"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"label":"foot"`) {
		t.Fatalf("json %q", out)
	}
	if strings.Contains(out, `"groups"`) {
		t.Fatal("json should be today only")
	}
}
