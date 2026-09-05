package look

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestScanSkipsHiddenAndAliasesPrism(t *testing.T) {
	apps, err := ScanDesktops([]string{desktopFixture(t)})
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) != 1 || apps[0].ID != "org.prismlauncher.PrismLauncher" || apps[0].WMClass != "PrismLauncher" {
		t.Fatalf("apps: %+v", apps)
	}
	res := Search("mine", ListFun, apps, Focus{}, nil, nil)
	if len(res.Hits) != 1 || res.Hits[0].Name != "Minecraft" {
		t.Fatalf("mine: %+v", res.Hits)
	}
	empty := Search("mine", ListFun, nil, Focus{}, nil, nil)
	if len(empty.Hits) != 0 {
		t.Fatalf("mine without prism: %+v", empty.Hits)
	}
}

func TestSearchPasteYoutubeAndOmitKhanOnFun(t *testing.T) {
	yt := Search("youtube.com", ListFun, nil, Focus{}, nil, nil)
	if len(yt.Hits) != 1 || yt.Hits[0].ID != "site:youtube.com" || yt.Hits[0].Name != "YouTube" {
		t.Fatalf("youtube.com: %+v", yt.Hits)
	}
	word := Search("youtube", ListFun, nil, Focus{}, nil, nil)
	if len(word.Hits) != 0 {
		t.Fatalf("youtube word: %+v", word.Hits)
	}
	khan := Search("khanacademy.org", ListFun, nil, Focus{}, nil, nil)
	if len(khan.Hits) != 0 {
		t.Fatalf("khan on fun: %+v", khan.Hits)
	}
	school := Search("khanacademy.org", ListSchool, nil, Focus{}, nil, nil)
	if len(school.Hits) != 1 || school.Hits[0].Name != "Khan Academy" {
		t.Fatalf("khan on school: %+v", school.Hits)
	}
}

func TestSearchFocusAndWinID(t *testing.T) {
	if WinID("PrismLauncher") == "win:PrismLauncher" {
		t.Fatal("raw class")
	}
	if WinID("PrismLauncher") != WinID("PrismLauncher") {
		t.Fatal("unstable")
	}
	always := func(c string) bool { return c == "omarchy-shell" }
	res := Search("", ListFun, nil, Focus{Class: "omarchy-shell", Title: "bar"}, always, nil)
	if res.Focus != nil {
		t.Fatalf("always_on: %+v", res.Focus)
	}
	res = Search("mine", ListFun, nil, Focus{Class: "PrismLauncher", Title: "Minecraft"}, nil, nil)
	if res.Focus == nil || res.Focus.Name != "Minecraft" || !res.Focus.Focused {
		t.Fatalf("focus: %+v", res.Focus)
	}
}

func TestCanFillNeedsClass(t *testing.T) {
	with := InstalledApp{ID: "x", Name: "X", WMClass: "X"}
	if MatcherForInstalled(with).ClassExact[0] != "X" {
		t.Fatal("wm class")
	}
	bare := InstalledApp{ID: "y", Name: "Y"}
	m := MatcherForInstalled(bare)
	if len(m.ClassExact) != 0 && m.ClassExact[0] != "" {
		t.Fatalf("bare exact: %+v", m)
	}
	if len(m.ClassRe) != 0 {
		t.Fatalf("bare re: %+v", m)
	}
}

func desktopFixture(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "testdata", "apps")
}
