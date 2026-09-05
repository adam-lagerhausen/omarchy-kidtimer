package look

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"unicode"
)

type Focus struct {
	Class string
	Title string
}

type Hit struct {
	Thing
	Source  string `json:"source,omitempty"`
	Focused bool   `json:"focused,omitempty"`
}

type SearchResult struct {
	Q      string
	Focus  *Hit
	Hits   []Hit
	Notice *string
}

func WinID(class string) string {
	sum := sha256.Sum256([]byte(class))
	return "win:" + hex.EncodeToString(sum[:8])
}

func NormalizeHost(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	if !strings.Contains(s, ".") {
		return "", false
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	if host == "" || !strings.Contains(host, ".") {
		return "", false
	}
	for _, r := range host {
		if r != '.' && r != '-' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return "", false
		}
	}
	return host, true
}

func Search(q string, list ListID, installed []InstalledApp, focus Focus, alwaysOn func(string) bool, listed map[string]string) SearchResult {
	out := SearchResult{Q: q, Hits: []Hit{}}
	if alwaysOn == nil {
		alwaysOn = func(string) bool { return false }
	}
	if listed == nil {
		listed = map[string]string{}
	}
	out.Focus = focusHit(q, list, installed, focus, alwaysOn, listed)
	if host, ok := NormalizeHost(q); ok {
		if !(list == ListFun && host == "khanacademy.org") {
			id := "site:" + host
			if listed[id] != string(list) {
				name := host
				for _, n := range Nicknames() {
					if n.Kind == ThingSite && n.Domain == host {
						name = n.Name
						break
					}
				}
				out.Hits = append(out.Hits, Hit{Thing: Thing{ID: id, Name: name, Kind: ThingSite}, Source: "site"})
			}
		}
	}
	for _, t := range AliasInstalled(q, installed, list) {
		if listed[t.ID] == string(list) {
			continue
		}
		if list == ListFun && t.ID == "site:khanacademy.org" {
			continue
		}
		out.Hits = append(out.Hits, Hit{Thing: t, Source: "desktop"})
		if len(out.Hits) >= 12 {
			break
		}
	}
	return out
}

func focusHit(q string, list ListID, installed []InstalledApp, focus Focus, alwaysOn func(string) bool, listed map[string]string) *Hit {
	class := strings.TrimSpace(focus.Class)
	if class == "" || alwaysOn(class) {
		return nil
	}
	hit := Hit{Focused: true, Source: "focus"}
	for _, n := range Nicknames() {
		m := Matcher{ClassRe: n.ClassRe, ClassExact: SiteClasses(n.Domain)}
		if !matcherHits(m, class, "") {
			continue
		}
		if list == ListFun && n.Prefer == ListSchool {
			continue
		}
		kind := n.Kind
		id := WinID(class)
		if n.Kind == ThingSite && n.Domain != "" {
			id = "site:" + n.Domain
			kind = ThingSite
		}
		if kind == ThingApp {
			for _, app := range installed {
				if strings.EqualFold(app.WMClass, class) {
					id = "desktop:" + strings.TrimSuffix(app.ID, ".desktop")
					break
				}
			}
		}
		if listed[id] == string(list) {
			return nil
		}
		hit.Thing = Thing{ID: id, Name: n.Name, Kind: kind}
		return &hit
	}
	for _, app := range installed {
		if app.WMClass != "" && strings.EqualFold(app.WMClass, class) {
			id := "desktop:" + strings.TrimSuffix(app.ID, ".desktop")
			if listed[id] == string(list) {
				return nil
			}
			name := app.Name
			nick := nickForInstalled(app)
			if nick.Name != "" {
				if list == ListFun && nick.Prefer == ListSchool {
					continue
				}
				name = nick.Name
			}
			hit.Thing = Thing{ID: id, Name: name, Kind: ThingApp}
			return &hit
		}
	}
	title := strings.TrimSpace(focus.Title)
	name := "This window"
	if q != "" {
		name = q
	} else if personFacing(title) {
		name = title
	}
	id := WinID(class)
	if listed[id] == string(list) {
		return nil
	}
	hit.Thing = Thing{ID: id, Name: name, Kind: ThingApp}
	return &hit
}

func personFacing(title string) bool {
	if title == "" || strings.Contains(title, ".") {
		return false
	}
	if strings.Contains(title, " ") {
		return true
	}
	for _, r := range title {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}
