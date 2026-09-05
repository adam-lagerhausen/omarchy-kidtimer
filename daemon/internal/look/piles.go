package look

import (
	"fmt"
	"strings"
)

type Piles struct {
	list []Pile
	apps map[string]string
}

func PilesFrom(piles []Pile, apps map[string]string) (Piles, error) {
	out := Piles{
		list: make([]Pile, 0, len(piles)),
		apps: map[string]string{},
	}
	seen := map[string]bool{}
	for _, p := range piles {
		if p.ID == "" {
			return Piles{}, fmt.Errorf("pile id is required")
		}
		if seen[p.ID] {
			return Piles{}, fmt.Errorf("duplicate pile %q", p.ID)
		}
		if strings.TrimSpace(p.Name) == "" {
			return Piles{}, fmt.Errorf("pile %q: name is required", p.ID)
		}
		if p.ID == string(ListSchool) {
			return Piles{}, fmt.Errorf("school is not a pile")
		}
		seen[p.ID] = true
		out.list = append(out.list, Pile{ID: p.ID, Name: p.Name})
	}
	for app, pile := range apps {
		if app == "" {
			return Piles{}, fmt.Errorf("app id is required")
		}
		if OldCatalogSlug(app) {
			continue
		}
		switch pile {
		case string(ListSchool):
			out.apps[app] = pile
		case string(ListFun):
			if !seen[pile] {
				return Piles{}, fmt.Errorf("app %q: unknown pile %q", app, pile)
			}
			out.apps[app] = pile
		default:
			return Piles{}, fmt.Errorf("app %q: unknown list %q", app, pile)
		}
	}
	return out, nil
}

func (p Piles) List() []Pile {
	out := make([]Pile, len(p.list))
	copy(out, p.list)
	return out
}

func (p Piles) Apps() map[string]string {
	out := make(map[string]string, len(p.apps))
	for k, v := range p.apps {
		out[k] = v
	}
	return out
}

func (p Piles) Sitting() []App {
	return nil
}

func (p Piles) PileOf(appID string) (string, bool) {
	id, ok := p.apps[appID]
	return id, ok
}

func (p Piles) Has(id string) bool {
	for _, pile := range p.list {
		if pile.ID == id {
			return true
		}
	}
	return false
}

func (p Piles) Move(appID, pileID string) (Piles, error) {
	if appID == "" {
		return Piles{}, fmt.Errorf("app id is required")
	}
	if pileID == "" {
		if _, ok := p.apps[appID]; !ok {
			return p, nil
		}
		next := p.clone()
		delete(next.apps, appID)
		return next, nil
	}
	if pileID != string(ListFun) && pileID != string(ListSchool) {
		return Piles{}, fmt.Errorf("unknown list %q", pileID)
	}
	if pileID == string(ListFun) && !p.Has(pileID) {
		return Piles{}, fmt.Errorf("unknown pile %q", pileID)
	}
	if p.apps[appID] == pileID {
		return p, nil
	}
	next := p.clone()
	next.apps[appID] = pileID
	return next, nil
}

func (p Piles) clone() Piles {
	return Piles{list: p.List(), apps: p.Apps()}
}
