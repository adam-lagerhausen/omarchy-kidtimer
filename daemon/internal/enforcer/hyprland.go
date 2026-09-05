package enforcer

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type hyprWindow struct {
	Class string `json:"class"`
	Title string `json:"title"`
	PID   int    `json:"pid"`
}

type Hyprland struct {
	UID int
}

func (h Hyprland) Active() (Window, bool, error) {
	if h.UID <= 0 {
		return Window{}, false, nil
	}
	runtimeDir := fmt.Sprintf("/run/user/%d", h.UID)
	sig := hyprSignature(runtimeDir)
	if sig == "" {
		return Window{}, false, nil
	}
	cmd := exec.Command("hyprctl", "activewindow", "-j")
	cmd.Env = append(os.Environ(),
		"XDG_RUNTIME_DIR="+runtimeDir,
		"HYPRLAND_INSTANCE_SIGNATURE="+sig,
	)
	out, err := cmd.Output()
	if err != nil {
		return Window{}, false, nil
	}
	var w hyprWindow
	if err := json.Unmarshal(out, &w); err != nil {
		return Window{}, false, nil
	}
	if w.Class == "" && w.PID == 0 {
		return Window{}, false, nil
	}
	return Window{Class: w.Class, Title: w.Title, PID: w.PID}, true, nil
}

type LogindSession struct {
	UID int
}

func (s LogindSession) Locked() bool {
	if s.UID <= 0 {
		return false
	}
	if omarchySessionLocked(s.UID) {
		return true
	}
	return loginctlBool(s.UID, "LockedHint")
}

func omarchySessionLocked(uid int) bool {
	runtimeDir := fmt.Sprintf("/run/user/%d", uid)
	cmd := exec.Command("omarchy-hyprland-session-locked")
	env := append(os.Environ(), "XDG_RUNTIME_DIR="+runtimeDir)
	if sig := hyprSignature(runtimeDir); sig != "" {
		env = append(env, "HYPRLAND_INSTANCE_SIGNATURE="+sig)
	}
	if display := waylandDisplay(runtimeDir); display != "" {
		env = append(env, "WAYLAND_DISPLAY="+display)
	}
	cmd.Env = env
	return cmd.Run() == nil
}

func (s LogindSession) Idle() bool {
	return loginctlBool(s.UID, "IdleHint")
}

func loginctlBool(uid int, prop string) bool {
	if uid <= 0 {
		return false
	}
	cmd := exec.Command("loginctl", "show-user", strconv.Itoa(uid), "-p", prop, "--value")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "yes"
}

func hyprSignature(runtimeDir string) string {
	entries, err := os.ReadDir(filepath.Join(runtimeDir, "hypr"))
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() {
			return e.Name()
		}
	}
	return ""
}
