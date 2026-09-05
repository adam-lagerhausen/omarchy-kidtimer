package look

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func DesktopDirs(home string) []string {
	dirs := []string{
		"/usr/share/applications",
		"/usr/local/share/applications",
		"/var/lib/flatpak/exports/share/applications",
	}
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local/share/applications"),
			filepath.Join(home, ".local/share/flatpak/exports/share/applications"),
		)
	}
	return dirs
}

func ScanDesktops(dirs []string) ([]InstalledApp, error) {
	var out []InstalledApp
	seen := map[string]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".desktop") {
				continue
			}
			app, ok, err := ParseDesktopFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			if !ok || seen[app.ID] {
				continue
			}
			seen[app.ID] = true
			out = append(out, app)
		}
	}
	return out, nil
}

func ParseDesktopFile(path string) (InstalledApp, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return InstalledApp{}, false, err
	}
	defer f.Close()
	id := strings.TrimSuffix(filepath.Base(path), ".desktop")
	var name, wmClass, typ string
	hidden := false
	noDisplay := false
	inDesktop := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			inDesktop = line == "[Desktop Entry]"
			continue
		}
		if !inDesktop {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		switch k {
		case "Name":
			name = v
		case "StartupWMClass":
			wmClass = v
		case "Type":
			typ = v
		case "Hidden":
			hidden = strings.EqualFold(v, "true")
		case "NoDisplay":
			noDisplay = strings.EqualFold(v, "true")
		}
	}
	if err := sc.Err(); err != nil {
		return InstalledApp{}, false, err
	}
	if hidden || noDisplay || typ != "Application" || name == "" {
		return InstalledApp{}, false, nil
	}
	return InstalledApp{ID: id, Name: name, WMClass: wmClass}, true, nil
}
