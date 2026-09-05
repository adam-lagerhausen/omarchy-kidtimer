package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseParentLabPackaging(t *testing.T) {
	cfg, err := ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KidName != "parent-lab" {
		t.Fatalf("kid_name: %q", cfg.KidName)
	}
	if cfg.Enforcer {
		t.Fatal("parent-lab enforcer must be off")
	}
	if cfg.BedtimeLock {
		t.Fatal("parent-lab bedtime_lock must be off")
	}
	if cfg.RemoteLock {
		t.Fatal("parent-lab remote_lock must be off")
	}
	if cfg.EmptyLock {
		t.Fatal("parent-lab empty_lock must be off")
	}
	if cfg.Advertise {
		t.Fatal("parent-lab advertise must be off")
	}
	if cfg.Listen != "127.0.0.1:8742" {
		t.Fatalf("parent-lab listen: %q", cfg.Listen)
	}
	if cfg.DefaultMode != "" {
		t.Fatal("parent-lab must not set default_mode")
	}
	if len(cfg.Schedule) != 0 {
		t.Fatal("parent-lab must not schedule modes")
	}
	if !cfg.Lab.Enabled {
		t.Fatal("parent-lab lab must be enabled")
	}
	if len(cfg.Lab.Classes) != 1 || cfg.Lab.Classes[0] != "kidtimer-lab" {
		t.Fatalf("parent-lab lab classes: %+v", cfg.Lab.Classes)
	}
	if len(cfg.Groups) != 1 || cfg.Groups[0].ID != "fun" {
		t.Fatalf("groups: %+v", cfg.Groups)
	}
	fun, _ := cfg.Group("fun")
	if fun.Policy != Metered || fun.DailySeconds != 3600 || fun.Overflow != "" {
		t.Fatalf("fun: %+v", fun)
	}
}

func TestParseKidPackaging(t *testing.T) {
	cfg, err := ParseFile(packagingPath(t, "config.kid.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enforcer || !cfg.BedtimeLock {
		t.Fatal("kid config should keep enforcer and bedtime_lock on")
	}
	if !cfg.EmptyLock {
		t.Fatal("kid empty_lock must be on")
	}
	if !cfg.RemoteLock {
		t.Fatal("kid remote_lock must be on")
	}
	if !cfg.Advertise {
		t.Fatal("kid advertise must be on")
	}
	if cfg.Listen != "0.0.0.0:8742" {
		t.Fatalf("kid listen: %q", cfg.Listen)
	}
	if cfg.Lab.Enabled {
		t.Fatal("kid lab must be off")
	}
	if len(cfg.Groups) != 1 || cfg.Groups[0].ID != "fun" {
		t.Fatalf("groups: %+v", cfg.Groups)
	}
}

func TestPackagingUnit(t *testing.T) {
	b, err := os.ReadFile(packagingPath(t, "kidtimer.service"))
	if err != nil {
		t.Fatal(err)
	}
	unit := string(b)
	for _, want := range []string{
		"After=network-online.target tailscaled.service",
		"Wants=tailscaled.service",
		"ExecStart=/usr/local/bin/kidtimer daemon",
		"Restart=on-failure",
		"StartLimitBurst=5",
		"StartLimitIntervalSec=60",
	} {
		if !strings.Contains(unit, want) {
			t.Fatalf("unit missing %q:\n%s", want, unit)
		}
	}
	if strings.Contains(unit, "enforcer") || strings.Contains(unit, "bedtime_lock") {
		t.Fatal("unit must not pass enforcer flags")
	}
}

func TestEmptyMatchIsValid(t *testing.T) {
	raw := `
kid_name = "x"
timezone = "UTC"
bedtime_start = "21:00"
bedtime_end = "07:00"
[groups.fun]
policy = "metered"
daily_seconds = 3600
[groups.school]
policy = "bypass"
`
	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	fun, ok := cfg.Group("fun")
	if !ok || len(fun.Match) != 0 {
		t.Fatalf("fun: %+v", fun)
	}
}

func TestEmptyKidNameIsHostname(t *testing.T) {
	host, err := os.Hostname()
	if err != nil || host == "" {
		t.Skip("hostname")
	}
	raw := `
timezone = "UTC"
bedtime_start = "21:00"
bedtime_end = "07:00"
[groups.fun]
policy = "metered"
daily_seconds = 3600
`
	cfg, err := Parse([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.KidName != host {
		t.Fatalf("kid_name: %q want %q", cfg.KidName, host)
	}
}

func TestModeMissingGroup(t *testing.T) {
	raw := mustReadParentLab(t) + `
[modes.broken]
nope = 0
`
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestModeBypassGroup(t *testing.T) {
	raw := `
kid_name = "x"
timezone = "UTC"
bedtime_start = "21:00"
bedtime_end = "07:00"
[groups.fun]
policy = "metered"
daily_seconds = 3600
[groups.school]
policy = "bypass"
[modes.broken]
school = 0
`
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestScheduleMissingMode(t *testing.T) {
	raw := mustReadParentLab(t) + `
[[schedule]]
days = ["mon"]
start = "08:00"
end = "10:00"
mode = "nope"
`
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestMatchFirstHitAndIgnoreUnknown(t *testing.T) {
	cfg := loadParentLab(t)
	id, ok := cfg.Match("kidtimer-lab", "")
	if !ok || id != "fun" {
		t.Fatalf("lab: %q %v", id, ok)
	}
	if _, ok := cfg.Match("PrismLauncher", ""); ok {
		t.Fatal("prism is not a TOML group")
	}
	if _, ok := cfg.Match("Alacritty", "zsh"); ok {
		t.Fatal("unknown windows must not match")
	}
}

func TestAlwaysOnEmptyClass(t *testing.T) {
	cfg := loadParentLab(t)
	if !cfg.AlwaysOnClass("") {
		t.Fatal("empty class is always-on")
	}
	if !cfg.AlwaysOnClass("omarchy-shell") {
		t.Fatal("shell is always-on")
	}
	if cfg.AlwaysOnClass("PrismLauncher") {
		t.Fatal("prism is not always-on")
	}
}

func TestBedtimeWrapsMidnight(t *testing.T) {
	cfg := loadParentLab(t)
	inside := time.Date(2026, 8, 26, 21, 0, 0, 0, cfg.Location)
	if !cfg.BedtimeContains(inside) {
		t.Fatal("21:00 should be inside")
	}
	late := time.Date(2026, 8, 27, 3, 0, 0, 0, cfg.Location)
	if !cfg.BedtimeContains(late) {
		t.Fatal("03:00 should be inside")
	}
	end := time.Date(2026, 8, 27, 7, 0, 0, 0, cfg.Location)
	if cfg.BedtimeContains(end) {
		t.Fatal("07:00 should be outside")
	}
	afternoon := time.Date(2026, 8, 26, 15, 0, 0, 0, cfg.Location)
	if cfg.BedtimeContains(afternoon) {
		t.Fatal("15:00 should be outside")
	}
}

func TestSecondsUntilBedtime(t *testing.T) {
	cfg := loadParentLab(t)
	afternoon := time.Date(2026, 8, 26, 15, 0, 0, 0, cfg.Location)
	if got := cfg.SecondsUntilBedtime(afternoon); got != 6*3600 {
		t.Fatalf("15:00: %d", got)
	}
	night := time.Date(2026, 8, 26, 22, 0, 0, 0, cfg.Location)
	if got := cfg.SecondsUntilBedtime(night); got != 0 {
		t.Fatalf("22:00: %d", got)
	}
	morning := time.Date(2026, 8, 27, 8, 0, 0, 0, cfg.Location)
	if got := cfg.SecondsUntilBedtime(morning); got != 13*3600 {
		t.Fatalf("08:00: %d", got)
	}
}

func TestOverflowMissing(t *testing.T) {
	raw := twoGroupRaw() + `
[groups.games]
policy = "metered"
daily_seconds = 3600
overflow = "bonus"
`
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestOverflowBypass(t *testing.T) {
	raw := twoGroupRaw() + `
[groups.games]
policy = "metered"
daily_seconds = 3600
overflow = "school"
`
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestOverflowSelf(t *testing.T) {
	raw := twoGroupRaw() + `
[groups.games]
policy = "metered"
daily_seconds = 3600
overflow = "games"
`
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestOverflowChain(t *testing.T) {
	raw := twoGroupRaw() + `
[groups.games]
policy = "metered"
daily_seconds = 3600
overflow = "fun"
`
	fun := strings.Replace(raw, `daily_seconds = 3600
`, `daily_seconds = 3600
overflow = "games"
`, 1)
	if _, err := Parse([]byte(fun)); err == nil {
		t.Fatal("expected parse error")
	}
}

func twoGroupRaw() string {
	return `
kid_name = "x"
timezone = "UTC"
bedtime_start = "21:00"
bedtime_end = "07:00"
[groups.fun]
policy = "metered"
daily_seconds = 3600
[groups.school]
policy = "bypass"
`
}

func TestRejectDotStarTitleMeter(t *testing.T) {
	raw := `
kid_name = "x"
timezone = "America/New_York"
bedtime_start = "21:00"
bedtime_end = "07:00"
[groups.youtube]
policy = "metered"
daily_seconds = 3600
match = [{ class = ".*", title = "(?i)youtube" }]
`
	if _, err := Parse([]byte(raw)); err == nil {
		t.Fatal("class = \".*\" with title must fail")
	}
}

func loadParentLab(t *testing.T) *Config {
	t.Helper()
	cfg, err := ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func mustReadParentLab(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func packagingPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	return filepath.Join(root, "packaging", name)
}
