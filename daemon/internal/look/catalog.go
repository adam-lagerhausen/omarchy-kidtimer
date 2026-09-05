package look

import (
	"regexp"
	"strings"
)

type App struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type pattern struct {
	Class *regexp.Regexp
	Title *regexp.Regexp
}

type catalogApp struct {
	App
	match pattern
}

func (a App) Caption() string {
	return strings.ToLower(a.Name)
}

var catalog = []catalogApp{
	mustApp("minecraft", "Minecraft", `(?i)minecraft|prismlauncher|org.prismlauncher`, ""),
	mustApp("roblox", "Roblox", `(?i)roblox`, ""),
	mustApp("youtube", "YouTube", `chrome-www.youtube.com__-Default`, ""),
	mustApp("steam", "Steam", `(?i)steam|steamwebhelper`, ""),
	mustApp("discord", "Discord", `(?i)discord`, ""),
	mustApp("spotify", "Spotify", `(?i)spotify`, ""),
	mustApp("chrome", "Chrome", `(?i)^google-chrome$`, ""),
	mustApp("twitch", "Twitch", `(?i)twitch`, ""),
	mustApp("epic", "Epic", `(?i)epicgames|unrealengine`, ""),
}

func mustApp(id, name, class, title string) catalogApp {
	a := catalogApp{App: App{ID: id, Name: name}}
	if class != "" {
		a.match.Class = regexp.MustCompile(class)
	}
	if title != "" {
		a.match.Title = regexp.MustCompile(title)
	}
	return a
}

func Catalog() []App {
	out := make([]App, len(catalog))
	for i, a := range catalog {
		out[i] = a.App
	}
	return out
}

func CatalogIDs() map[string]App {
	out := make(map[string]App, len(catalog))
	for _, a := range catalog {
		out[a.ID] = a.App
	}
	return out
}

func Match(class, title string) (App, bool) {
	for _, a := range catalog {
		if patternHit(a.match, class, title) {
			return a.App, true
		}
	}
	return App{}, false
}

func patternHit(p pattern, class, title string) bool {
	if p.Class == nil && p.Title == nil {
		return false
	}
	if p.Class != nil && !p.Class.MatchString(class) {
		return false
	}
	if p.Title != nil && !p.Title.MatchString(title) {
		return false
	}
	return true
}

func knownApp(id string) bool {
	_, ok := CatalogIDs()[id]
	return ok
}

func OldCatalogSlug(id string) bool {
	_, ok := CatalogIDs()[id]
	return ok
}
