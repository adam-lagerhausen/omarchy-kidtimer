package session

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

func GraphicalUID() (int, error) {
	if uid, err := UIDFromLoginctl(); err == nil {
		return uid, nil
	}
	return UIDFromHyprRoots("/run/user")
}

func Home(uid int) (string, error) {
	u, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return "", err
	}
	return u.HomeDir, nil
}

func UIDFromLoginctl() (int, error) {
	out, err := exec.Command("loginctl", "list-sessions", "--no-legend").Output()
	if err != nil {
		return 0, err
	}
	return ParseLoginctl(string(out))
}

func ParseLoginctl(raw string) (int, error) {
	var fallback int
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 3 {
			continue
		}
		uid, err := strconv.Atoi(fields[1])
		if err != nil || uid == 0 {
			continue
		}
		line := strings.ToLower(sc.Text())
		graphical := strings.Contains(line, "wayland") || strings.Contains(line, "x11") || strings.Contains(line, "tty")
		if !graphical {
			continue
		}
		if strings.Contains(line, "active") {
			return uid, nil
		}
		if fallback == 0 {
			fallback = uid
		}
	}
	if fallback != 0 {
		return fallback, nil
	}
	return 0, fmt.Errorf("no graphical session")
}

func UIDFromHyprRoots(runUser string) (int, error) {
	entries, err := os.ReadDir(runUser)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		uid, err := strconv.Atoi(e.Name())
		if err != nil || uid == 0 {
			continue
		}
		if _, err := os.Stat(filepath.Join(runUser, e.Name(), "hypr")); err == nil {
			return uid, nil
		}
	}
	return 0, fmt.Errorf("no hypr session")
}
