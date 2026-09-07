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
		"manifest.json",
		"BarWidget.qml",
		"KidBar.qml",
		"KidPanel.qml",
		"KidModel.js",
		"ParentBar.qml",
		"ParentPanel.qml",
		"Tape.qml",
		"ParentModel.js",
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

	kidBar := readPlugin(t, root, "KidBar.qml") + readPlugin(t, root, "BarWidget.qml")
	kidPanel := readPlugin(t, root, "KidPanel.qml")
	kidModel := readPlugin(t, root, "KidModel.js")
	parentBar := readPlugin(t, root, "ParentBar.qml") + readPlugin(t, root, "BarWidget.qml")
	parentPanel := readPlugin(t, root, "ParentPanel.qml") + readPlugin(t, root, "Tape.qml")
	parentModel := readPlugin(t, root, "ParentModel.js")
	kidManifest := readPlugin(t, root, "manifest.json")
	parentManifest := readPlugin(t, root, "manifest.json")

	mustContain(t, kidBar, "http://127.0.0.1:8742", "localhost default")
	mustContain(t, kidBar, "/v1/status", "GET /v1/status")
	mustContain(t, kidBar, "omarchy-notification-send", "kid remaining warning")
	mustContain(t, kidBar, "takeWarnings", "warning crossings")
	mustContain(t, kidModel, `"kidtimer"`, "fallback title")
	mustContain(t, kidModel, `"bedtime"`, "bedtime label")
	mustContain(t, kidModel, `"fun"`, "default fun")
	mustContain(t, kidModel, "function panelShowsAsk", "ask on bedtime and lock")
	mustContain(t, kidModel, "function overlayCancel", "leave parent pin")
	mustContain(t, kidModel, "return [900, 300, 60]", "15/5/1 min warnings")
	mustContain(t, kidPanel, "POST", "kid ask POST")
	mustContain(t, kidPanel, "/v1/asks", "kid ask path")
	mustContain(t, kidPanel, "askToken", "ask token")
	mustContain(t, kidPanel, "Ask", "Ask")
	mustContain(t, kidPanel, "Ask for more", "ask sheet")
	mustContain(t, kidPanel, "−5", "ask nudge")
	mustContain(t, kidPanel, "Parent Pin", "waiting parent pin")
	mustContain(t, kidPanel, "panelShowsAsk", "ask stays on bedtime and lock")
	mustContain(t, kidPanel, "/v1/pin/approve", "pin approve")
	mustContain(t, kidPanel, "pendingAskId", "saved ask id")
	mustContain(t, kidPanel, "JetBrainsMono", "kid panel mono")
	kidOverlay := readPlugin(t, root, "Overlay.qml")
	mustContain(t, kidOverlay, "Parent Pin", "overlay parent pin")
	mustContain(t, kidOverlay, "/v1/pin/grant", "pin grant")
	mustContain(t, kidOverlay, "Ask", "overlay ask")
	mustContain(t, kidOverlay, "cancelPin", "leave parent pin")
	mustContain(t, kidOverlay, "overlayCancel", "reset pin step")
	mustContain(t, kidOverlay, "/v1/asks", "overlay ask path")
	mustContain(t, kidOverlay, "−10", "overlay ask nudge down")
	mustContain(t, kidOverlay, "+10", "overlay ask nudge up")
	mustContain(t, kidOverlay, "WlrLayer.Overlay", "overlay layer")
	mustContain(t, kidOverlay, "stay-awake", "idle inhibit")
	mustContain(t, kidOverlay, "submap", "super submap")
	mustContain(t, kidOverlay, "JetBrainsMono", "overlay mono")
	mustContain(t, kidOverlay, "Key_Escape", "escape does nothing")
	if strings.Contains(kidOverlay, `"overlay"`) && strings.Contains(kidManifest, `"overlay"`) {
		t.Fatal("do not add overlay kind")
	}
	if strings.Contains(readPlugin(t, root, "KidBar.qml")+kidPanel+kidModel, "/v1/lock") {
		t.Fatal("kid plugin must not post /v1/lock")
	}
	if strings.Contains(readPlugin(t, root, "KidBar.qml")+kidPanel, "Unlock") {
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
	mustContain(t, parentBar, `running: root.role === ""`, "poll role until the file appears")
	mustContain(t, parentBar, `running: root.role === "parent" && snapshots.length === 0`, "poll household until the first kid appears")
	mustContain(t, parentBar, `running: root.role === "parent" && !root.householdPinSet`, "poll parent-pin until it appears")
	mustContain(t, readPlugin(t, root, "Overlay.qml"), `running: root.role === ""`, "overlay polls role until the file appears")
	mustContain(t, parentBar, `"parent"`, "start kidtimer parent")
	mustContain(t, parentBar, "kidtimerBin", "parent binary helper")
	mustContain(t, parentBar, "/usr/bin/bash", "parent setup via bash")
	mustContain(t, parentBar, "helperPath", "resolve apply-role path")
	mustContain(t, parentBar, "finishSetup", "clear setup busy")
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
	mustContain(t, parentPanel, "JetBrainsMono", "mono font")
	mustContain(t, parentPanel, "Color.popups", "popup surface")
	mustContain(t, parentPanel, "Color.urgent", "urgent token")
	mustContain(t, parentPanel, `"deny"`, "deny decision")
	mustContain(t, parentPanel, "body.implicitHeight", "kid switch stays inside the panel")
	mustContain(t, parentModel, "Parent Pin", "settings pin row")
	mustContain(t, parentModel, "Required for the controls. Use it to make changes on the kids computer.", "pin why")
	mustContain(t, parentPanel, "Change", "pin change")
	if strings.Contains(readPlugin(t, root, "ParentPanel.qml"), "onOpenedChanged") {
		t.Fatal("ParentPanel is an Item; opened lives on the host widget")
	}
	applyRole := readPlugin(t, root, "helpers/apply-role.sh")
	archIdx := strings.Index(applyRole, "kidtimer-linux-$want")
	hereIdx := strings.Index(applyRole, `"$here/kidtimer"`)
	destIdx := strings.Index(applyRole, `elf_ok "$dest"`)
	if archIdx < 0 || hereIdx < 0 || destIdx < 0 || archIdx > hereIdx || hereIdx > destIdx {
		t.Fatal("apply-role must prefer the matching plugin binary over a leftover ~/.local/bin/kidtimer")
	}
	mustContain(t, applyRole, "kidtimer-linux-$want", "arch plugin binary name")
	mustContain(t, applyRole, "setup-error", "kid setup error file")
	mustContain(t, applyRole, "od -An -t x1 -j 18 -N 2", "ELF machine check")
	mustContain(t, parentBar, "setup-error", "watch kid setup error")
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
	mustContain(t, readPlugin(t, root, "Panel.qml"), "Panel {", "shared panel")
	mustContain(t, readPlugin(t, root, "Panel.qml"), "Finishing setup", "setup loading copy")
	mustContain(t, readPlugin(t, root, "Panel.qml"), "RotationAnimation", "setup spinner")
	mustContain(t, readPlugin(t, root, "Panel.qml"), "setupBusy", "setup loading until done")
	mustContain(t, readPlugin(t, root, "Panel.qml"), "onHostWidgetChanged", "push host into setup after inject")
	mustContain(t, readPlugin(t, root, "Setup.qml"), "roleHost", "setup click finds host if inject was late")
	if strings.Contains(readPlugin(t, root, "Overlay.qml"), "textFormat: Text.PlainText anchors") {
		t.Fatal("overlay Text properties must be one per line")
	}
	mustContain(t, parentManifest, `"bar-widget"`, "bar-widget")
	mustContain(t, parentManifest, `"kidtimer"`, "one plugin id")
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
