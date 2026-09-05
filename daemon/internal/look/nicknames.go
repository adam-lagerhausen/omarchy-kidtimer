package look

import (
	"regexp"
	"strings"
)

type Nickname struct {
	Query   []string
	Name    string
	Kind    ThingKind
	Desktop []string
	ClassRe []string
	Domain  string
	Prefer  ListID
}

type InstalledApp struct {
	ID      string
	Name    string
	WMClass string
}

func Nicknames() []Nickname {
	return []Nickname{
		{Query: []string{"mine", "minecraft", "prism"}, Name: "Minecraft", Kind: ThingApp, Desktop: []string{"org.prismlauncher.PrismLauncher", "minecraft"}, ClassRe: []string{`(?i)minecraft|prismlauncher|org.prismlauncher`}},
		{Query: []string{"roblox"}, Name: "Roblox", Kind: ThingApp, Desktop: []string{"org.roblox.RobloxPlayer", "roblox-player"}, ClassRe: []string{`(?i)roblox`}},
		{Query: []string{"youtube", "yt"}, Name: "YouTube", Kind: ThingSite, Domain: "youtube.com"},
		{Query: []string{"steam"}, Name: "Steam", Kind: ThingApp, Desktop: []string{"steam"}, ClassRe: []string{`(?i)steam|steamwebhelper`}},
		{Query: []string{"discord"}, Name: "Discord", Kind: ThingApp, Desktop: []string{"discord", "com.discordapp.Discord"}, ClassRe: []string{`(?i)discord`}},
		{Query: []string{"spotify"}, Name: "Spotify", Kind: ThingApp, Desktop: []string{"spotify", "com.spotify.Client"}, ClassRe: []string{`(?i)spotify`}},
		{Query: []string{"chrome", "google chrome"}, Name: "Chrome", Kind: ThingApp, Desktop: []string{"google-chrome", "com.google.Chrome"}, ClassRe: []string{`(?i)^google-chrome$`}},
		{Query: []string{"chromium"}, Name: "Chromium", Kind: ThingApp, Desktop: []string{"chromium", "org.chromium.Chromium"}, ClassRe: []string{`(?i)^chromium$`}},
		{Query: []string{"twitch"}, Name: "Twitch", Kind: ThingApp, Desktop: []string{"twitch"}, ClassRe: []string{`(?i)twitch`}},
		{Query: []string{"epic", "fortnite"}, Name: "Epic", Kind: ThingApp, Desktop: []string{"com.epicgames.launcher", "epic"}, ClassRe: []string{`(?i)epicgames|unrealengine`}},
		{Query: []string{"khan", "khan academy"}, Name: "Khan Academy", Kind: ThingSite, Domain: "khanacademy.org", Prefer: ListSchool},
	}
}

func SiteClasses(host string) []string {
	if host == "" {
		return nil
	}
	return []string{
		"chrome-www." + host + "__-Default",
		"chrome-" + host + "__-Default",
		"chromium-www." + host + "__-Default",
		"chromium-" + host + "__-Default",
	}
}

func Resolve(doc Document, class, title string) (Thing, bool) {
	for _, t := range doc.Things {
		m, ok := doc.Matchers[t.ID]
		if !ok {
			continue
		}
		if matcherHits(m, class, title) {
			return t, true
		}
	}
	return Thing{}, false
}

func matcherHits(m Matcher, class, title string) bool {
	_ = title
	for _, exact := range m.ClassExact {
		if strings.EqualFold(exact, class) {
			return true
		}
	}
	for _, pat := range m.ClassRe {
		re, err := regexp.Compile(pat)
		if err != nil {
			continue
		}
		if re.MatchString(class) {
			return true
		}
	}
	return false
}

func AliasInstalled(q string, installed []InstalledApp, list ListID) []Thing {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}
	var hits []Thing
	seen := map[string]bool{}
	for _, app := range installed {
		if app.ID == "" || app.Name == "" {
			continue
		}
		nick := nickForInstalled(app)
		if list == ListFun && nick.Prefer == ListSchool {
			continue
		}
		if !nameMatchesQuery(q, app.Name, nick) {
			continue
		}
		id := "desktop:" + strings.TrimSuffix(app.ID, ".desktop")
		if seen[id] {
			continue
		}
		seen[id] = true
		name := app.Name
		if nick.Name != "" {
			name = nick.Name
		}
		hits = append(hits, Thing{ID: id, Name: name, Kind: ThingApp})
		if len(hits) >= 12 {
			break
		}
	}
	return hits
}

func MatcherForInstalled(app InstalledApp) Matcher {
	m := Matcher{DesktopID: strings.TrimSuffix(app.ID, ".desktop")}
	if app.WMClass != "" {
		m.ClassExact = []string{app.WMClass}
	}
	nick := nickForInstalled(app)
	m.ClassRe = append([]string{}, nick.ClassRe...)
	return m
}

func nickForInstalled(app InstalledApp) Nickname {
	id := strings.TrimSuffix(app.ID, ".desktop")
	for _, n := range Nicknames() {
		if n.Kind != ThingApp {
			continue
		}
		for _, d := range n.Desktop {
			if strings.EqualFold(d, id) {
				return n
			}
		}
		for _, pat := range n.ClassRe {
			re, err := regexp.Compile(pat)
			if err != nil {
				continue
			}
			if app.WMClass != "" && re.MatchString(app.WMClass) {
				return n
			}
		}
	}
	return Nickname{}
}

func nameMatchesQuery(q, name string, nick Nickname) bool {
	ql := strings.ToLower(q)
	if strings.Contains(strings.ToLower(name), ql) {
		return true
	}
	if nick.Name != "" && strings.Contains(strings.ToLower(nick.Name), ql) {
		return true
	}
	for _, query := range nick.Query {
		if strings.Contains(strings.ToLower(query), ql) || strings.Contains(ql, strings.ToLower(query)) {
			return true
		}
	}
	return false
}
