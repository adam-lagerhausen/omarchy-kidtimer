package look

import (
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"kidtimer/daemon/internal/config"
)

func TestPilesFromRejectsDuplicatesAndUnknown(t *testing.T) {
	piles := []Pile{{ID: "fun", Name: "Fun"}}
	if _, err := PilesFrom(piles, map[string]string{"desktop:prism": "fun", "site:khanacademy.org": "school"}); err != nil {
		t.Fatal(err)
	}
	if _, err := PilesFrom(append(piles, Pile{ID: "fun", Name: "Fun"}), nil); err == nil {
		t.Fatal("duplicate pile id")
	}
	if _, err := PilesFrom(piles, map[string]string{"desktop:prism": "nope"}); err == nil {
		t.Fatal("unknown list")
	}
	if _, err := PilesFrom([]Pile{{ID: "school", Name: "School"}}, nil); err == nil {
		t.Fatal("school pile")
	}
	if _, err := PilesFrom(nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMoveAppExclusiveAndDelete(t *testing.T) {
	doc := seedKid(t)
	doc.Things = []Thing{
		{ID: "desktop:prism", Name: "Minecraft", Kind: ThingApp},
		{ID: "site:khanacademy.org", Name: "Khan Academy", Kind: ThingSite},
	}
	doc.Matchers = map[string]Matcher{
		"desktop:prism": {ClassExact: []string{"PrismLauncher"}},
	}
	moved, err := doc.MoveApp("desktop:prism", "fun")
	if err != nil {
		t.Fatal(err)
	}
	if moved.Apps["desktop:prism"] != "fun" {
		t.Fatalf("fun: %q", moved.Apps["desktop:prism"])
	}
	school, err := moved.MoveApp("desktop:prism", "school")
	if err != nil {
		t.Fatal(err)
	}
	if school.Apps["desktop:prism"] != "school" {
		t.Fatal("exclusive school")
	}
	if school.Apps["desktop:prism"] == "fun" {
		t.Fatal("still on fun")
	}
	gone, err := school.MoveApp("desktop:prism", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := gone.Apps["desktop:prism"]; ok {
		t.Fatal("delete left apps key")
	}
	if _, ok := gone.Matchers["desktop:prism"]; ok {
		t.Fatal("delete left matcher")
	}
	for _, th := range gone.Things {
		if th.ID == "desktop:prism" {
			t.Fatal("delete left thing")
		}
	}
}

func TestDayFromRejectsOverlapWrapAndBedtime(t *testing.T) {
	bed := Bedtime{LightsOut: 1260, Duration: 600}
	ok, err := DayFrom([]Block{
		{ID: "morning", Start: 480, Duration: 120, Mode: "morning"},
		{ID: "homework", Start: 900, Duration: 120, Mode: "homework"},
	}, bed)
	if err != nil {
		t.Fatal(err)
	}
	if _, hit := ok.Covering(480); !hit {
		t.Fatal("8:00 should be morning")
	}
	if _, hit := ok.Covering(600); hit {
		t.Fatal("10:00 is a covering gap")
	}
	if _, hit := ok.Covering(601); hit {
		t.Fatal("10:01 is a covering gap")
	}
	if _, err := DayFrom([]Block{
		{ID: "a", Start: 480, Duration: 120, Mode: "morning"},
		{ID: "b", Start: 540, Duration: 60, Mode: "homework"},
	}, bed); err == nil {
		t.Fatal("overlap")
	}
	if _, err := DayFrom([]Block{
		{ID: "wrap", Start: 1380, Duration: 120, Mode: "evening"},
	}, bed); err == nil {
		t.Fatal("wrap")
	}
	if _, err := DayFrom([]Block{
		{ID: "night", Start: 1200, Duration: 120, Mode: "evening"},
	}, bed); err == nil {
		t.Fatal("bedtime intersection")
	}
	if _, err := DayFrom([]Block{
		{ID: "short", Start: 480, Duration: 10, Mode: "morning"},
	}, bed); err == nil {
		t.Fatal("short block")
	}
	touch, err := DayFrom([]Block{
		{ID: "eve", Start: 1020, Duration: 240, Mode: "evening"},
	}, bed)
	if err != nil {
		t.Fatal(err)
	}
	if touch.Blocks()[0].End() != 1260 {
		t.Fatal("block may touch lights out")
	}
}

func TestCoveringGapIsNull(t *testing.T) {
	doc := seedKid(t)
	loc := time.Date(2026, 8, 24, 10, 1, 0, 0, time.UTC).Location()
	_ = loc
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 8, 24, 10, 1, 0, 0, ny)
	if _, ok := doc.CoveringAt(at); ok {
		t.Fatal("monday 10:01 must be an empty gap")
	}
	morning := time.Date(2026, 8, 24, 8, 30, 0, 0, ny)
	if _, ok := doc.CoveringAt(morning); ok {
		t.Fatal("seed has no schedule blocks")
	}
}

func TestHostFaceRankAndLED(t *testing.T) {
	down := HostFaceFrom(false, true, true, "minecraft")
	if down.Kind != HostDown || down.LED() || down.Caption() != "down" {
		t.Fatalf("down: %+v led=%v cap=%q", down, down.LED(), down.Caption())
	}
	bed := HostFaceFrom(true, true, true, "minecraft")
	if bed.Kind != HostBedtime || bed.LED() || bed.Caption() != "bedtime" {
		t.Fatalf("bedtime: %+v", bed)
	}
	locked := HostFaceFrom(true, false, true, "minecraft")
	if locked.Kind != HostLocked || locked.LED() || locked.Caption() != "locked" {
		t.Fatalf("locked must beat on: %+v led=%v", locked, locked.LED())
	}
	on := HostFaceFrom(true, false, false, "minecraft")
	if on.Kind != HostOn || !on.LED() || on.Caption() != "on minecraft" {
		t.Fatalf("on: %+v led=%v cap=%q", on, on.LED(), on.Caption())
	}
	idle := HostFaceFrom(true, false, false, "")
	if idle.Kind != HostIdle || idle.LED() || idle.Caption() != "" {
		t.Fatalf("idle: %+v", idle)
	}
}

func TestCatalogMatchAndSittingAround(t *testing.T) {
	app, ok := Match("PrismLauncher", "")
	if !ok || app.ID != "minecraft" || app.Caption() != "minecraft" {
		t.Fatalf("prism: %+v %v", app, ok)
	}
	app, ok = Match("chrome-www.youtube.com__-Default", "")
	if !ok || app.ID != "youtube" {
		t.Fatalf("youtube: %+v %v", app, ok)
	}
	if _, ok := Match("chrome-www.khanacademy.org__-Default", ""); ok {
		t.Fatal("khan is not a catalog app")
	}
	if _, ok := Match("Alacritty", "zsh"); ok {
		t.Fatal("unknown class")
	}
	doc := seedParentLab(t)
	if len(doc.Apps) != 0 {
		t.Fatalf("seed apps: %+v", doc.Apps)
	}
	if len(doc.Sitting()) != 0 {
		t.Fatalf("sitting dump: %v", sittingIDs(doc))
	}
}

func TestSeedKeepsPileIdsAndDropsOverflow(t *testing.T) {
	doc := seedParentLab(t)
	if doc.Version != 2 {
		t.Fatalf("version: %d", doc.Version)
	}
	if len(doc.Piles) != 1 || doc.Piles[0].ID != "fun" || doc.Piles[0].Name != "Fun" {
		t.Fatalf("piles: %+v", doc.Piles)
	}
	if len(doc.Things) != 0 || len(doc.Apps) != 0 || len(doc.Matchers) != 0 {
		t.Fatalf("membership: things=%+v apps=%+v matchers=%+v", doc.Things, doc.Apps, doc.Matchers)
	}
	if doc.PileHours["fun"] != 3600 {
		t.Fatalf("pile hours: %+v", doc.PileHours)
	}
	if doc.FunHours[DaySat] != 7200 || doc.FunHours[DayMon] != 3600 {
		t.Fatalf("fun hours: %+v", doc.FunHours)
	}
	if doc.Bedtime.LightsOut != 21*60 || doc.Bedtime.Duration != 10*60 || doc.Bedtime.Wake() != 7*60 {
		t.Fatalf("bedtime: %+v wake=%d", doc.Bedtime, doc.Bedtime.Wake())
	}
}

func TestSeedKidScheduleBlocks(t *testing.T) {
	doc := seedKid(t)
	for _, id := range DayIDs {
		if len(doc.Schedule[id]) != 0 {
			t.Fatalf("%s should be empty: %+v", id, doc.Schedule[id])
		}
	}
}

func TestDocumentRejectsBedtimeTooShort(t *testing.T) {
	doc := seedParentLab(t)
	doc.Bedtime.Duration = 30
	if err := doc.Validate(); err == nil {
		t.Fatal("short bedtime")
	}
}

func TestCatalogNamesNeverCarryPatterns(t *testing.T) {
	for _, a := range Catalog() {
		if a.ID == "" || a.Name == "" {
			t.Fatalf("catalog: %+v", a)
		}
	}
	if len(Catalog()) != 9 {
		t.Fatalf("catalog len %d", len(Catalog()))
	}
}

func TestCloneKeepsMatchers(t *testing.T) {
	doc := seedKid(t)
	doc.Things = []Thing{{ID: "win:aaaaaaaaaaaaaaaa", Name: "This window", Kind: ThingApp}}
	doc.Apps = map[string]string{"win:aaaaaaaaaaaaaaaa": "fun"}
	doc.Matchers = map[string]Matcher{
		"win:aaaaaaaaaaaaaaaa": {ClassExact: []string{"PrismLauncher"}},
	}
	if err := doc.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := doc.Clone()
	got := cloned.Matchers["win:aaaaaaaaaaaaaaaa"]
	if len(got.ClassExact) != 1 || got.ClassExact[0] != "PrismLauncher" {
		t.Fatalf("clone matchers: %+v", cloned.Matchers)
	}
}

func TestEqualPolicySeesMatcherAndFunHours(t *testing.T) {
	doc := seedKid(t)
	doc.Things = []Thing{{ID: "desktop:prism", Name: "Minecraft", Kind: ThingApp}}
	doc.Apps = map[string]string{"desktop:prism": "fun"}
	doc.Matchers = map[string]Matcher{"desktop:prism": {ClassExact: []string{"PrismLauncher"}}}
	same := doc.Clone()
	if !doc.EqualPolicy(same) {
		t.Fatal("same policy")
	}
	edited := doc.Clone()
	m := edited.Matchers["desktop:prism"]
	m.ClassRe = []string{"(?i)minecraft"}
	edited.Matchers["desktop:prism"] = m
	if doc.EqualPolicy(edited) {
		t.Fatal("matcher edit is policy")
	}
	hours := doc.Clone()
	hours.FunHours[DaySat] = 3600
	if doc.EqualPolicy(hours) {
		t.Fatal("saturday hours are policy")
	}
}

func TestValidateDropsCatalogSlugs(t *testing.T) {
	doc := seedKid(t)
	doc.Apps = map[string]string{"minecraft": "fun", "desktop:prism": "fun"}
	doc.Things = []Thing{{ID: "desktop:prism", Name: "Minecraft", Kind: ThingApp}}
	if err := doc.Validate(); err != nil {
		t.Fatal(err)
	}
	cloned := doc.Clone()
	if _, ok := cloned.Apps["minecraft"]; ok {
		t.Fatal("dropped slug stayed")
	}
	if cloned.Apps["desktop:prism"] != "fun" {
		t.Fatalf("kept thing: %+v", cloned.Apps)
	}
}

func TestAdoptEmptiesClassicMembership(t *testing.T) {
	raw := []byte(`{"version":1,"piles":[{"id":"minecraft","name":"Games"},{"id":"fun","name":"Fun"}],"apps":{"minecraft":"minecraft","steam":"fun"},"pile_hours":{"fun":3600},"fun_hours":{"sat":7200},"modes":[{"id":"evening","name":"evening","hours":{"fun":0,"minecraft":0}}],"schedule":{"mon":[],"tue":[],"wed":[],"thu":[],"fri":[],"sat":[],"sun":[]},"bedtime":{"lights_out":1260,"duration":600}}`)
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if !AdoptPersisted(&doc) {
		t.Fatal("expected adopt")
	}
	if doc.Version != 2 {
		t.Fatalf("version: %d", doc.Version)
	}
	if len(doc.Things) != 0 || len(doc.Apps) != 0 || len(doc.Matchers) != 0 {
		t.Fatalf("membership: %+v %+v %+v", doc.Things, doc.Apps, doc.Matchers)
	}
	if doc.Bedtime.LightsOut != 1260 || doc.PileHours["fun"] != 3600 || doc.FunHours["sat"] != 7200 {
		t.Fatalf("kept clocks: bedtime=%+v hours=%+v fun=%+v", doc.Bedtime, doc.PileHours, doc.FunHours)
	}
}

func TestAliasMineNeedsPrismDesktop(t *testing.T) {
	if hits := AliasInstalled("mine", nil, ListFun); len(hits) != 0 {
		t.Fatalf("no desktop: %v", hits)
	}
	prism := InstalledApp{ID: "org.prismlauncher.PrismLauncher.desktop", Name: "Prism Launcher", WMClass: "PrismLauncher"}
	hits := AliasInstalled("mine", []InstalledApp{prism}, ListFun)
	if len(hits) != 1 || hits[0].ID != "desktop:org.prismlauncher.PrismLauncher" || hits[0].Name != "Minecraft" {
		t.Fatalf("prism alias: %+v", hits)
	}
	doc := seedKid(t)
	if len(doc.Apps) != 0 {
		t.Fatal("alias must not write membership")
	}
	m := MatcherForInstalled(prism)
	if len(m.ClassExact) != 1 || m.ClassExact[0] != "PrismLauncher" {
		t.Fatalf("exact: %+v", m)
	}
	if len(m.ClassRe) == 0 {
		t.Fatal("copied ClassRe")
	}
}

func TestResolveSpendMatching(t *testing.T) {
	doc := seedKid(t)
	prism := InstalledApp{ID: "org.prismlauncher.PrismLauncher", Name: "Prism Launcher", WMClass: "PrismLauncher"}
	doc.Things = []Thing{
		{ID: "desktop:org.prismlauncher.PrismLauncher", Name: "Minecraft", Kind: ThingApp},
		{ID: "desktop:google-chrome", Name: "Chrome", Kind: ThingApp},
		{ID: "site:youtube.com", Name: "YouTube", Kind: ThingSite},
	}
	doc.Apps = map[string]string{
		"desktop:org.prismlauncher.PrismLauncher": "fun",
		"desktop:google-chrome":                   "fun",
		"site:youtube.com":                        "fun",
	}
	doc.Matchers = map[string]Matcher{
		"desktop:org.prismlauncher.PrismLauncher": MatcherForInstalled(prism),
		"desktop:google-chrome": {
			ClassExact: []string{"Google-chrome"},
			ClassRe:    []string{`(?i)^google-chrome$`},
		},
		"site:youtube.com": {
			Domain:     "youtube.com",
			ClassExact: SiteClasses("youtube.com"),
		},
	}
	if err := doc.Validate(); err != nil {
		t.Fatal(err)
	}
	got, ok := Resolve(doc, "PrismLauncher", "")
	if !ok || got.Name != "Minecraft" {
		t.Fatalf("prism: %+v %v", got, ok)
	}
	got, ok = Resolve(doc, "minecraft", "")
	if !ok || got.ID != "desktop:org.prismlauncher.PrismLauncher" {
		t.Fatalf("class minecraft after prism pick: %+v %v", got, ok)
	}
	got, ok = Resolve(doc, "google-chrome", "")
	if !ok || got.Name != "Chrome" {
		t.Fatalf("chrome: %+v %v", got, ok)
	}
	got, ok = Resolve(doc, "chrome-www.youtube.com__-Default", "")
	if !ok || got.Name != "YouTube" {
		t.Fatalf("youtube app class: %+v %v", got, ok)
	}
	if _, ok := Resolve(seedKid(t), "PrismLauncher", ""); ok {
		t.Fatal("unlisted prism must sit around")
	}
}

func seedParentLab(t *testing.T) Document {
	t.Helper()
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Seed(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func seedKid(t *testing.T) Document {
	t.Helper()
	cfg, err := config.ParseFile(packagingPath(t, "config.kid.toml"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Seed(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func sittingIDs(doc Document) []string {
	var ids []string
	for _, a := range doc.Sitting() {
		ids = append(ids, a.ID)
	}
	return ids
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
