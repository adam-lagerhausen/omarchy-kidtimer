package look

import (
	"fmt"
	"strings"
)

type Fill struct {
	Installed []InstalledApp
	WinClass  map[string]string
}

func ApplyIncoming(stored, incoming Document, fill Fill) (Document, error) {
	out := incoming.Clone()
	if len(incoming.FunHours) == 0 {
		out.FunHours = stored.Clone().FunHours
	}
	out.Matchers = map[string]Matcher{}
	installed := map[string]InstalledApp{}
	for _, app := range fill.Installed {
		installed[strings.TrimSuffix(app.ID, ".desktop")] = app
	}
	for _, t := range out.Things {
		if m, ok := stored.Matchers[t.ID]; ok {
			out.Matchers[t.ID] = m
			continue
		}
		m, err := fillMatcher(t, fill, installed)
		if err != nil {
			return Document{}, err
		}
		out.Matchers[t.ID] = m
	}
	if err := out.Validate(); err != nil {
		return Document{}, err
	}
	return out, nil
}

func fillMatcher(t Thing, fill Fill, installed map[string]InstalledApp) (Matcher, error) {
	switch {
	case strings.HasPrefix(t.ID, "nick:"):
		return Matcher{}, fmt.Errorf("unknown thing %q", t.ID)
	case strings.HasPrefix(t.ID, "desktop:"):
		id := strings.TrimPrefix(t.ID, "desktop:")
		app, ok := installed[id]
		if !ok {
			return Matcher{}, fmt.Errorf("unknown thing %q", t.ID)
		}
		m := MatcherForInstalled(app)
		if len(m.ClassExact) == 0 && len(m.ClassRe) == 0 {
			return Matcher{}, fmt.Errorf("unknown thing %q", t.ID)
		}
		return m, nil
	case strings.HasPrefix(t.ID, "site:"):
		host := strings.TrimPrefix(t.ID, "site:")
		if host == "" {
			return Matcher{}, fmt.Errorf("unknown thing %q", t.ID)
		}
		return Matcher{Domain: host, ClassExact: SiteClasses(host)}, nil
	case strings.HasPrefix(t.ID, "win:"):
		class := fill.WinClass[t.ID]
		if class == "" {
			return Matcher{}, fmt.Errorf("unknown thing %q", t.ID)
		}
		return Matcher{ClassExact: []string{class}}, nil
	default:
		if OldCatalogSlug(t.ID) {
			return Matcher{}, nil
		}
		return Matcher{}, fmt.Errorf("unknown thing %q", t.ID)
	}
}
