package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginQMLIsHTTPClientNotBank(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		"plugin-kid/manifest.json",
		"plugin-kid/BarWidget.qml",
		"plugin-kid/Panel.qml",
		"plugin-kid/KidModel.js",
		"plugin-parent/manifest.json",
		"plugin-parent/BarWidget.qml",
		"plugin-parent/Panel.qml",
		"plugin-parent/Tape.qml",
		"plugin-parent/ParentModel.js",
	}
	joined := ""
	for _, rel := range files {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		joined += rel + "\n" + string(b) + "\n"
	}

	for _, banned := range []string{
		"sqlite",
		"/var/lib/kidtimer",
		"/etc/kidtimer",
		"hyprctl",
		"SIGSTOP",
		"omarchy system lock",
	} {
		if strings.Contains(joined, banned) {
			t.Fatalf("plugins must not contain %q", banned)
		}
	}

	kidBar := readPlugin(t, root, "plugin-kid/BarWidget.qml")
	kidPanel := readPlugin(t, root, "plugin-kid/Panel.qml")
	kidModel := readPlugin(t, root, "plugin-kid/KidModel.js")
	parentBar := readPlugin(t, root, "plugin-parent/BarWidget.qml")
	parentPanel := readPlugin(t, root, "plugin-parent/Panel.qml") + readPlugin(t, root, "plugin-parent/Tape.qml")
	parentModel := readPlugin(t, root, "plugin-parent/ParentModel.js")
	kidManifest := readPlugin(t, root, "plugin-kid/manifest.json")
	parentManifest := readPlugin(t, root, "plugin-parent/manifest.json")

	mustContain(t, kidBar, "http://127.0.0.1:8742", "localhost default")
	mustContain(t, kidBar, "/v1/status", "GET /v1/status")
	mustContain(t, kidBar, "omarchy-notification-send", "kid remaining warning")
	mustContain(t, kidBar, "--exec", "click warning opens plugin")
	mustContain(t, kidBar, "summon", "summon kid plugin")
	mustContain(t, kidBar, "takeWarnings", "warning crossings")
	mustContain(t, kidModel, `"kidtimer"`, "fallback title")
	mustContain(t, kidModel, `"bedtime"`, "bedtime label")
	mustContain(t, kidModel, `"fun"`, "default fun")
	mustContain(t, kidModel, "return [900, 300, 60]", "15/5/1 min warnings")
	mustContain(t, kidPanel, "POST", "kid ask POST")
	mustContain(t, kidPanel, "/v1/asks", "kid ask path")
	mustContain(t, kidPanel, "askToken", "ask token")
	mustContain(t, kidPanel, "Ask", "Ask")
	mustContain(t, kidPanel, "Ask for more", "ask sheet")
	mustContain(t, kidPanel, "−5", "ask nudge")
	mustContain(t, kidPanel, "Parent Pin", "waiting parent pin")
	mustContain(t, kidPanel, "/v1/pin/approve", "pin approve")
	mustContain(t, kidPanel, "pendingAskId", "saved ask id")
	kidOverlay := readPlugin(t, root, "plugin-kid/Overlay.qml")
	mustContain(t, kidOverlay, "Parent Pin", "overlay parent pin")
	mustContain(t, kidOverlay, "/v1/pin/grant", "pin grant")
	mustContain(t, kidOverlay, "Ask", "overlay ask")
	mustContain(t, kidOverlay, "/v1/asks", "overlay ask path")
	mustContain(t, kidOverlay, "−10", "overlay ask nudge down")
	mustContain(t, kidOverlay, "+10", "overlay ask nudge up")
	mustContain(t, kidOverlay, "WlrLayer.Overlay", "overlay layer")
	mustContain(t, kidOverlay, "stay-awake", "idle inhibit")
	mustContain(t, kidOverlay, "submap", "super submap")
	mustContain(t, kidOverlay, "IBM Plex Mono", "overlay plex")
	mustContain(t, kidOverlay, "Key_Escape", "escape does nothing")
	if strings.Contains(kidOverlay, `"overlay"`) && strings.Contains(kidManifest, `"overlay"`) {
		t.Fatal("do not add overlay kind")
	}
	if strings.Contains(kidBar+kidPanel+kidModel, "/v1/lock") {
		t.Fatal("kid plugin must not post /v1/lock")
	}
	if strings.Contains(kidBar+kidPanel, "Unlock") {
		t.Fatal("kid plugin must not offer unlock")
	}

	mustContain(t, parentBar, "/v1/status", "parent status")
	mustContain(t, parentBar, "/v1/asks", "parent asks poll")
	mustContain(t, parentBar, "/v1/look", "parent look")
	mustContain(t, parentBar, "interval: 5000", "asks poll 5s")
	mustContain(t, parentBar, "/v1/grants", "parent grant")
	mustContain(t, parentBar, "omarchy-notification-send", "omarchy notification")
	mustContain(t, parentBar, "newAskIds", "pending id diff")
	mustContain(t, parentBar, `"fun"`, "default fun")
	mustContain(t, parentBar, `"/v1/asks/"`, "parent decide path")
	mustContain(t, parentBar, `"/v1/lock"`, "parent lock")
	mustContain(t, parentBar, `"PATCH"`, "parent policy")
	mustContain(t, parentBar, `"/v1/policy"`, "parent policy path")
	mustContain(t, parentBar, `"PUT"`, "parent look put")
	mustContain(t, parentBar, "kids.json", "household file")
	mustContain(t, parentBar, "FileView", "watch kids.json")
	mustContain(t, parentBar, `"parent"`, "start kidtimer parent")
	mustContain(t, parentBar, "kidtimerBin", "parent binary helper")
	mustContain(t, parentBar, "/v1/household", "desk household")
	mustContain(t, parentBar, "/v1/adopt", "desk adopt")
	mustContain(t, parentBar, "/v1/kids/", "desk kid routes")
	mustContain(t, parentBar, "sendKid", "paired kid policy via desk")
	mustContain(t, parentModel, "mergeKids", "merge pins and pair")
	mustContain(t, parentModel, "function parentBin", "parentBin helper")
	mustContain(t, parentModel, "function kidtimerBin", "kidtimerBin helper")
	mustContain(t, parentModel, ".local/bin/kidtimer", "parent binary path")
	mustContain(t, kidBar, `"kid"`, "start kidtimer kid")
	mustContain(t, kidModel, "function kidtimerBin", "kid kidtimerBin")
	mustContain(t, kidModel, "/usr/local/bin/kidtimer", "kid system binary")
	if strings.Contains(parentBar+parentPanel+parentModel, "/v1/mode") {
		t.Fatal("parent plugin must not call /v1/mode")
	}
	if strings.Contains(parentBar+parentPanel+parentModel, "/v1/pair") {
		t.Fatal("parent plugin must not pair")
	}
	if strings.Contains(parentBar+parentPanel+parentModel, "/v1/reclaim") {
		t.Fatal("parent plugin must not reclaim")
	}
	mustContain(t, parentPanel, "+10", "+10")
	mustContain(t, parentPanel, "−10", "minus 10")
	mustContain(t, parentPanel, "USED", "used minutes")
	mustContain(t, parentPanel, "LEFT", "fun left")
	mustContain(t, parentPanel, "APPROVE", "approve")
	mustContain(t, parentPanel, "DENY", "deny")
	mustContain(t, parentPanel, "BED", "bed clock")
	mustContain(t, parentPanel, "LOCKED", "locked stamp")
	mustContain(t, parentPanel, "IBMPlexMono", "plex font")
	mustContain(t, parentPanel, "Color.popups", "popup surface")
	mustContain(t, parentPanel, "Color.urgent", "urgent token")
	mustContain(t, parentPanel, `"deny"`, "deny decision")
	mustContain(t, parentModel, "Parent Pin", "settings pin row")
	mustContain(t, parentModel, "Required for the controls. Use it to make changes on the kids computer.", "pin why")
	mustContain(t, parentPanel, "Change", "pin change")
	mustContain(t, parentPanel, "model: track.blocks", "activity on the track")
	mustContain(t, parentPanel, "color: root.accent", "activity uses theme accent")
	mustContain(t, parentPanel, "function blockLeft", "track grows min width left of now")
	mustContain(t, parentModel, "start_unix", "parent reads UTC sit start")
	mustContain(t, parentModel, "if (start > cap) continue", "occupancy clips to now")
	mustContain(t, parentBar, `"pin"`, "pin set cli")
	mustContain(t, parentBar, `"approve"`, "approve decision")
	if strings.Contains(parentPanel, "GIVE 10") || strings.Contains(parentBar, "giveTen") {
		t.Fatal("ask buttons are deny/approve, not give 10")
	}
	mustContain(t, parentModel, "Lock ", "lock")
	mustContain(t, parentModel, "Unlock ", "unlock")
	mustContain(t, parentModel, `"fun"`, "model default fun")
	mustContain(t, parentModel, "http://127.0.0.1:8742", "localhost default")
	mustContain(t, parentModel, "fixtureTape", "fixture tape")
	mustContain(t, parentModel, "function clockLabel", "clockLabel helper")
	mustContain(t, parentModel, "function parsePrefs", "prefs helper")
	mustContain(t, parentPanel, `kind: "clock"`, "clock format row")
	mustContain(t, parentBar, "prefs.json", "clock prefs file")
	mustContain(t, kidModel, "hour12", "kid hour12")
	if strings.Contains(parentPanel, "edit groups") || strings.Contains(parentPanel, "edit schedule") {
		t.Fatal("till tape must not contain edit groups or edit schedule")
	}
	if strings.Contains(parentPanel, "KeepLastFrame") || strings.Contains(parentPanel, "eagle.webm") {
		t.Fatal("till tape must not contain the free-time eagle")
	}
	if strings.Contains(parentPanel, "#1daeeb") {
		t.Fatal("till tape must not use Omarchy cyan")
	}
	for _, banned := range []string{"#f3e4c4", "#2a120e", "#b42318", "#8a6a3a", "#d4b484"} {
		if strings.Contains(parentPanel, banned) {
			t.Fatalf("till tape must not contain leftover cream/brown %q", banned)
		}
	}

	mustContain(t, kidManifest, `"bar-widget"`, "kid bar-widget")
	mustContain(t, kidManifest, `"service"`, "kid overlay service")
	mustContain(t, kidManifest, `"keepLoaded"`, "keep overlay loaded")
	mustContain(t, kidManifest, "Overlay.qml", "overlay entry")
	if strings.Contains(kidManifest, `"overlay"`) {
		t.Fatal("do not add overlay kind")
	}
	mustContain(t, kidPanel, "Panel {", "kid panel")
	mustContain(t, parentManifest, `"bar-widget"`, "parent bar-widget")
	if strings.Contains(parentManifest, `"service"`) {
		t.Fatal("parent plugin must not be a Quickshell bank service")
	}
}

func TestPluginModels(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	script := filepath.Join(repoRoot(t), "testdata", "run-plugin-models.js")
	cmd := exec.Command(node, script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin models: %v\n%s", err, out)
	}
	if strings.TrimSpace(string(out)) != "ok" {
		t.Fatalf("plugin models: %s", out)
	}
}

func TestDaemonRefreshesSessionUID(t *testing.T) {
	src := readPlugin(t, repoRoot(t), "daemon/cmd/kidtimer/main.go")
	if strings.Contains(src, "uid = os.Getuid()") {
		t.Fatal("graphical uid must not fall back to root")
	}
	mustContain(t, src, "GraphicalUID", "per-tick session uid")
	mustContain(t, src, "SessionUID", "enforcer session uid")
	mustContain(t, src, "InputIdle", "input idle watch")
	mustContain(t, src, "enforcer:", "log tick errors")
}

func readPlugin(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustContain(t *testing.T, src, needle, label string) {
	t.Helper()
	if !strings.Contains(src, needle) {
		t.Fatalf("%s: missing %q", label, needle)
	}
}
