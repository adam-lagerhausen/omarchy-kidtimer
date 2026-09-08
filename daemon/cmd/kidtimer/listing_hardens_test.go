package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoopbackHTTPBodyOffArgv(t *testing.T) {
	ln, addr := listenLoopbackHTTP(t)
	defer func() {
		if ln != nil {
			_ = ln.Close()
		}
	}()
	const (
		token = "loopback-token-9f3a"
		body  = `{"reason":"body-off-argv-secret-9f3a"}`
	)
	got := captureHTTP(t, ln, func() {
		cmd := exec.Command(filepath.Join(repoRoot(t), "helpers", "loopback-http.sh"), "POST", "http://"+addr+"/v1/grants")
		cmd.Stdin = strings.NewReader(token + "\n" + body)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("loopback-http: %v\n%s", err, out)
		}
	})
	if got.body != body {
		t.Fatalf("server body %q", got.body)
	}
	if got.auth != "Bearer "+token {
		t.Fatalf("auth %q", got.auth)
	}
	if len(got.cmdlines) == 0 {
		t.Fatal("curl not seen")
	}
	for _, line := range got.cmdlines {
		if strings.Contains(line, "body-off-argv-secret-9f3a") {
			t.Fatalf("http body on argv: %s", line)
		}
		if strings.Contains(line, token) {
			t.Fatalf("bearer on argv: %s", line)
		}
		if !strings.Contains(line, "/usr/bin/curl") {
			t.Fatalf("unpinned curl: %s", line)
		}
		if !strings.Contains(line, "--data-binary") || !strings.Contains(line, "@-") {
			t.Fatalf("body not on stdin: %s", line)
		}
	}
}

func TestTestdataGrantBodyOffArgv(t *testing.T) {
	srv := httptestOnLocal(t)
	defer srv.Close()
	const token = "grant-token-9f3a"
	got := captureHTTP(t, srv.Listener, func() {
		cmd := exec.Command(filepath.Join(repoRoot(t), "testdata", "grant.sh"))
		cmd.Env = append(os.Environ(),
			"KIDTIMER_URL="+srv.URL,
			"KIDTIMER_TOKEN="+token,
			"IDEMPOTENCY_KEY=testdata-grant-1",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("grant.sh: %v\n%s", err, out)
		}
	})
	if !strings.Contains(got.body, "good afternoon") {
		t.Fatalf("server body %q", got.body)
	}
	if got.auth != "Bearer "+token {
		t.Fatalf("auth %q", got.auth)
	}
	if len(got.cmdlines) == 0 {
		t.Fatal("curl not seen")
	}
	for _, line := range got.cmdlines {
		if strings.Contains(line, "good afternoon") || strings.Contains(line, `"group":"fun"`) {
			t.Fatalf("http body on argv: %s", line)
		}
		if strings.Contains(line, token) {
			t.Fatalf("bearer on argv: %s", line)
		}
		if !strings.Contains(line, "/usr/bin/curl") {
			t.Fatalf("unpinned curl: %s", line)
		}
		if !strings.Contains(line, "-q") {
			t.Fatalf("curl missing -q: %s", line)
		}
		if !strings.Contains(line, "--max-time") || !strings.Contains(line, "--max-filesize") {
			t.Fatalf("curl missing caps: %s", line)
		}
		if !strings.Contains(line, "--data-binary") || !strings.Contains(line, "@-") {
			t.Fatalf("body not on stdin: %s", line)
		}
	}
}

func TestTestdataStatusPinnedCurl(t *testing.T) {
	srv := httptestOnLocal(t)
	defer srv.Close()
	const token = "status-token-9f3a"
	got := captureHTTP(t, srv.Listener, func() {
		cmd := exec.Command(filepath.Join(repoRoot(t), "testdata", "status.sh"))
		cmd.Env = append(os.Environ(),
			"KIDTIMER_URL="+srv.URL,
			"KIDTIMER_TOKEN="+token,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("status.sh: %v\n%s", err, out)
		}
	})
	if got.auth != "Bearer "+token {
		t.Fatalf("auth %q", got.auth)
	}
	if len(got.cmdlines) == 0 {
		t.Fatal("curl not seen")
	}
	for _, line := range got.cmdlines {
		if strings.Contains(line, token) {
			t.Fatalf("bearer on argv: %s", line)
		}
		if !strings.Contains(line, "/usr/bin/curl") {
			t.Fatalf("unpinned curl: %s", line)
		}
		if !strings.Contains(line, "--max-time") || !strings.Contains(line, "--max-filesize") {
			t.Fatalf("curl missing caps: %s", line)
		}
	}
}

func TestApplyRoleWritesSetupError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"id":"io.github.adam-lagerhausen.kidtimer","version":"1.0.0"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "packaging"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "packaging", "config.kid.toml"), []byte("kid_name = \"x\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "helpers"), 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join(repoRoot(t), "helpers", "apply-role.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "helpers", "apply-role.sh")
	if err := os.WriteFile(script, src, 0o755); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(dir, "home")
	cmd := exec.Command(script, "parent")
	cmd.Env = []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin",
		"TMPDIR=" + dir,
	}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected setup fail: %s", out)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".local", "share", "kidtimer", "setup-error"))
	if err != nil {
		t.Fatalf("setup-error missing (%v): %s", err, out)
	}
	if !strings.Contains(string(raw), "Need the kidtimer binary") {
		t.Fatalf("setup-error %q out %s", raw, out)
	}
	st, err := os.Stat(filepath.Join(home, ".local", "share", "kidtimer", "setup-error"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}
}

func TestApplyRoleSetupErrorDoesNotFollowSymlink(t *testing.T) {
	dir := t.TempDir()
	share := filepath.Join(dir, "share")
	if err := os.MkdirAll(share, 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(dir, "victim")
	if err := os.WriteFile(victim, []byte("must survive\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(share, "setup-error")); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(repoRoot(t), "helpers", "apply-role.sh")
	cmd := exec.Command("bash", "-c", `
set -euo pipefail
share=$1
eval "$(sed -n '/^fail()/,/^}/p' "$2")"
fail "need the kidtimer binary"
`, "apply-role-fail", share, script)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("fail should exit 1: %s", out)
	}
	got, err := os.ReadFile(victim)
	if err != nil || string(got) != "must survive\n" {
		t.Fatalf("wrote through setup-error symlink: %s %v", got, err)
	}
	st, err := os.Lstat(filepath.Join(share, "setup-error"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSymlink != 0 {
		t.Fatal("setup-error still a symlink")
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %o", st.Mode().Perm())
	}
	msg, err := os.ReadFile(filepath.Join(share, "setup-error"))
	if err != nil || strings.TrimSpace(string(msg)) != "need the kidtimer binary" {
		t.Fatalf("setup-error %s %v", msg, err)
	}
}

type httpCapture struct {
	body     string
	auth     string
	cmdlines []string
}

func captureHTTP(t *testing.T, ln net.Listener, run func()) httpCapture {
	t.Helper()
	ch := make(chan httpCapture, 1)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ch <- httpCapture{
			body:     string(b),
			auth:     r.Header.Get("Authorization"),
			cmdlines: curlCmdlines(r.Host, r.URL.Path),
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	})}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	run()
	select {
	case got := <-ch:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("no http request")
		return httpCapture{}
	}
}

func httptestOnLocal(t *testing.T) *localServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return &localServer{Listener: ln, URL: "http://" + ln.Addr().String()}
}

type localServer struct {
	Listener net.Listener
	URL      string
}

func (s *localServer) Close() {
	_ = s.Listener.Close()
}

func listenLoopbackHTTP(t *testing.T) (net.Listener, string) {
	t.Helper()
	for _, addr := range []string{"127.0.0.1:8742", "127.0.0.1:8741"} {
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			return ln, addr
		}
	}
	t.Skip("loopback 8741/8742 in use")
	return nil, ""
}

func curlCmdlines(host, path string) []string {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		name := e.Name()
		if name == "" || name[0] < '0' || name[0] > '9' {
			continue
		}
		raw, err := os.ReadFile("/proc/" + name + "/cmdline")
		if err != nil || len(raw) == 0 {
			continue
		}
		line := strings.TrimSpace(strings.ReplaceAll(string(raw), "\x00", " "))
		if !strings.Contains(line, "/usr/bin/curl") {
			continue
		}
		if host != "" && strings.Contains(line, host) {
			out = append(out, line)
			continue
		}
		if path != "" && strings.Contains(line, path) {
			out = append(out, line)
		}
	}
	return out
}
