package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"kidtimer/daemon/internal/household"
)

func TestParseDirectFromFlags(t *testing.T) {
	clearLabEnv(t)
	req, err := Parse("grant", []string{"-url", "http://127.0.0.1:9", "-token", "secret"})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := req.Target.(Direct)
	if !ok {
		t.Fatalf("target %T", req.Target)
	}
	if d.URL != "http://127.0.0.1:9" {
		t.Fatalf("url %s", d.URL)
	}
	if d.Token.s != "secret" {
		t.Fatal("token not stored")
	}
	if d.Token.String() != "" || d.Token.GoString() != "token{}" {
		t.Fatal("token must not print")
	}
	if strings.Contains(fmt.Sprintf("%v", d), "secret") || strings.Contains(fmt.Sprintf("%#v", d), "secret") {
		t.Fatal("token leaked")
	}
}

func TestParseDirectFromEnv(t *testing.T) {
	t.Setenv("KIDTIMER_URL", "http://127.0.0.1:9")
	t.Setenv("KIDTIMER_TOKEN", "env-secret")
	req, err := Parse("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := req.Target.(Direct)
	if !ok {
		t.Fatalf("target %T", req.Target)
	}
	if d.URL != "http://127.0.0.1:9" || d.Token.s != "env-secret" {
		t.Fatalf("direct %+v", d)
	}
}

func TestParseDeskDefault(t *testing.T) {
	clearLabEnv(t)
	req, err := Parse("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := req.Target.(Desk)
	if !ok {
		t.Fatalf("target %T", req.Target)
	}
	if d.URL != defaultDeskURL {
		t.Fatalf("desk %s", d.URL)
	}
	if d.Kid != "" {
		t.Fatalf("kid %s", d.Kid)
	}
}

func TestParseURLWithoutToken(t *testing.T) {
	clearLabEnv(t)
	if _, err := Parse("status", []string{"-url", "http://127.0.0.1:9"}); err == nil {
		t.Fatal("url without token")
	}
}

func TestParseTokenWithoutURL(t *testing.T) {
	clearLabEnv(t)
	if _, err := Parse("status", []string{"-token", "secret"}); err == nil {
		t.Fatal("token without url")
	}
}

func TestParseTokenFlagBeatsEnv(t *testing.T) {
	t.Setenv("KIDTIMER_URL", "http://127.0.0.1:9")
	t.Setenv("KIDTIMER_TOKEN", "env-secret")
	req, err := Parse("status", []string{"-url", "http://127.0.0.1:8", "-token", "flag-secret"})
	if err != nil {
		t.Fatal(err)
	}
	d, ok := req.Target.(Direct)
	if !ok || d.URL != "http://127.0.0.1:8" || d.Token.s != "flag-secret" {
		t.Fatalf("direct %+v", req.Target)
	}
}

func TestParseKidWithDirect(t *testing.T) {
	clearLabEnv(t)
	_, err := Parse("lock", []string{"-url", "http://127.0.0.1:9", "-token", "secret", "-kid", "Ada"})
	if err == nil {
		t.Fatal("-kid with Direct")
	}
}

func TestParseDecidePositionals(t *testing.T) {
	clearLabEnv(t)
	req, err := Parse("decide", []string{"abc", "approve"})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := req.Action.(Decide)
	if !ok || a.ID != "abc" || a.Decision != decisionApprove {
		t.Fatalf("decide %+v", req.Action)
	}
	req, err = Parse("decide", []string{"-url", "http://127.0.0.1:9", "-token", "secret", "xyz", "deny"})
	if err != nil {
		t.Fatal(err)
	}
	a, ok = req.Action.(Decide)
	if !ok || a.ID != "xyz" || a.Decision != decisionDeny {
		t.Fatalf("decide %+v", req.Action)
	}
	if _, err := Parse("decide", []string{"abc"}); err == nil {
		t.Fatal("decide missing decision")
	}
}

func TestDeskLockNoBearerAndPinGate(t *testing.T) {
	clearLabEnv(t)
	var posts int
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization %q on %s", r.Header.Get("Authorization"), r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/household":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"kids":[{"id":"kid-1","name":"Ada","live":true}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/kids/kid-1/lock":
			posts++
			gotAuth = r.Header.Get("Authorization")
			gotPath = r.URL.Path
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"locked":true}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	req, err := Parse("lock", []string{"-desk", srv.URL, "-kid", "Ada", "-home", home})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := captureStdout(t, func() error { return Do(req) }); err == nil || !strings.Contains(err.Error(), "PIN") {
		t.Fatalf("PIN missing: %v", err)
	}
	if posts != 0 {
		t.Fatalf("posted before PIN: %d", posts)
	}
	if err := household.WritePin(home, "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := captureStdout(t, func() error { return Do(req) }); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatalf("posts %d", posts)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization %q", gotAuth)
	}
	if gotPath != "/v1/kids/kid-1/lock" {
		t.Fatalf("path %s", gotPath)
	}
}

func TestDeskGrantPath(t *testing.T) {
	clearLabEnv(t)
	var gotAuth, gotPath, gotIdem string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization %q on %s", r.Header.Get("Authorization"), r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/household":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"kids":[{"id":"kid-1","name":"Ada","live":true}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/kids/kid-1/grants":
			gotAuth = r.Header.Get("Authorization")
			gotPath = r.URL.Path
			gotIdem = r.Header.Get("Idempotency-Key")
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"group":"fun","seconds":60}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	req, err := Parse("grant", []string{
		"-desk", srv.URL, "-kid", "Ada",
		"-seconds", "60", "-reason", "+1", "-idempotency-key", "test-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := captureStdout(t, func() error { return Do(req) }); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/kids/kid-1/grants" {
		t.Fatalf("path %s", gotPath)
	}
	if gotIdem != "test-key" {
		t.Fatalf("Idempotency-Key %q", gotIdem)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization %q", gotAuth)
	}
}

func clearLabEnv(t *testing.T) {
	t.Helper()
	t.Setenv("KIDTIMER_URL", "")
	t.Setenv("KIDTIMER_TOKEN", "")
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	runErr := fn()
	_ = w.Close()
	os.Stdout = old
	b, _ := io.ReadAll(r)
	_ = r.Close()
	return string(b), runErr
}
