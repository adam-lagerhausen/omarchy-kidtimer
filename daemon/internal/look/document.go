package look

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	DayMon = "mon"
	DayTue = "tue"
	DayWed = "wed"
	DayThu = "thu"
	DayFri = "fri"
	DaySat = "sat"
	DaySun = "sun"
)

var DayIDs = []string{DayMon, DayTue, DayWed, DayThu, DayFri, DaySat, DaySun}

type ModeKind string

const (
	KindAllotment ModeKind = "allotment"
	KindFreetime  ModeKind = "freetime"
	FreetimeID             = "freetime"
)

type ThingKind string

const (
	ThingApp  ThingKind = "app"
	ThingSite ThingKind = "site"
)

type Thing struct {
	ID   string    `json:"id"`
	Name string    `json:"name"`
	Kind ThingKind `json:"kind"`
}

type ListID string

const (
	ListFun    ListID = "fun"
	ListSchool ListID = "school"
)

type Matcher struct {
	DesktopID  string   `json:"desktop_id,omitempty"`
	ClassExact []string `json:"class_exact,omitempty"`
	ClassRe    []string `json:"class_re,omitempty"`
	Domain     string   `json:"domain,omitempty"`
}

type Document struct {
	Version        int                `json:"version"`
	Piles          []Pile             `json:"piles"`
	Things         []Thing            `json:"things"`
	Apps           map[string]string  `json:"apps"`
	Matchers       map[string]Matcher `json:"matchers"`
	PileHours      map[string]int     `json:"pile_hours"`
	FunHours       map[string]int     `json:"fun_hours"`
	Modes          []Mode             `json:"modes"`
	Schedule       map[string][]Block `json:"schedule"`
	Bedtime        Bedtime            `json:"bedtime"`
	StickyFreetime bool               `json:"sticky_freetime"`
}

type Pile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Mode struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Kind  ModeKind       `json:"kind"`
	Hours map[string]int `json:"hours"`
}

func CanonicalFreetime() Mode {
	return Mode{ID: FreetimeID, Name: "Freetime", Kind: KindFreetime, Hours: map[string]int{}}
}

type Bedtime struct {
	LightsOut int `json:"lights_out"`
	Duration  int `json:"duration"`
}

func (d Document) Validate() error {
	d.normalize()
	if err := d.Bedtime.Validate(); err != nil {
		return err
	}
	piles, err := PilesFrom(d.Piles, d.Apps)
	if err != nil {
		return err
	}
	if err := d.validateThings(); err != nil {
		return err
	}
	ids := pileIDSet(piles.List())
	if err := validateHoursMap("pile_hours", d.PileHours, ids, false); err != nil {
		return err
	}
	seenMode := map[string]bool{}
	freetimeN := 0
	for _, m := range d.Modes {
		if m.ID == "" {
			return fmt.Errorf("mode id is required")
		}
		if seenMode[m.ID] {
			return fmt.Errorf("duplicate mode %q", m.ID)
		}
		seenMode[m.ID] = true
		if strings.TrimSpace(m.Name) == "" {
			return fmt.Errorf("mode %q: name is required", m.ID)
		}
		switch m.Kind {
		case KindFreetime:
			if m.ID != FreetimeID {
				return fmt.Errorf("freetime mode id must be %q", FreetimeID)
			}
			if len(m.Hours) != 0 {
				return fmt.Errorf("mode %q: freetime hours must be empty", m.ID)
			}
			freetimeN++
		case KindAllotment:
			if m.ID == FreetimeID {
				return fmt.Errorf("mode %q: allotment cannot use freetime id", m.ID)
			}
			if err := validateHoursMap("mode "+m.ID, m.Hours, ids, true); err != nil {
				return err
			}
		default:
			return fmt.Errorf("mode %q: unknown kind %q", m.ID, m.Kind)
		}
	}
	if freetimeN != 1 {
		return fmt.Errorf("look must contain the freetime mode")
	}
	for _, dayID := range DayIDs {
		blocks := d.Schedule[dayID]
		day, err := DayFrom(blocks, d.Bedtime)
		if err != nil {
			return fmt.Errorf("schedule %s: %w", dayID, err)
		}
		for _, b := range day.Blocks() {
			if !seenMode[b.Mode] {
				return fmt.Errorf("schedule %s: unknown mode %q", dayID, b.Mode)
			}
		}
	}
	for dayID := range d.Schedule {
		if !validDayID(dayID) {
			return fmt.Errorf("schedule: unknown day %q", dayID)
		}
	}
	return nil
}

func validateHoursMap(label string, hours map[string]int, piles map[string]bool, complete bool) error {
	for id, sec := range hours {
		if !piles[id] {
			return fmt.Errorf("%s: unknown pile %q", label, id)
		}
		if sec < 0 {
			return fmt.Errorf("%s: pile %q seconds must be >= 0", label, id)
		}
	}
	if complete {
		for id := range piles {
			if _, ok := hours[id]; !ok {
				return fmt.Errorf("%s: missing pile %q", label, id)
			}
		}
	}
	return nil
}

func pileIDSet(piles []Pile) map[string]bool {
	out := make(map[string]bool, len(piles))
	for _, p := range piles {
		out[p.ID] = true
	}
	return out
}

func validDayID(id string) bool {
	for _, d := range DayIDs {
		if d == id {
			return true
		}
	}
	return false
}

func (d Document) Clone() Document {
	raw, err := json.Marshal(d)
	if err != nil {
		panic(err)
	}
	var out Document
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	out.normalize()
	return out
}

func (d *Document) normalize() {
	if d.Apps == nil {
		d.Apps = map[string]string{}
	}
	if d.Matchers == nil {
		d.Matchers = map[string]Matcher{}
	}
	if d.PileHours == nil {
		d.PileHours = map[string]int{}
	}
	if d.FunHours == nil {
		d.FunHours = map[string]int{}
	}
	if d.Schedule == nil {
		d.Schedule = map[string][]Block{}
	}
	if d.Things == nil {
		d.Things = []Thing{}
	}
	for _, id := range DayIDs {
		if d.Schedule[id] == nil {
			d.Schedule[id] = []Block{}
		}
	}
	for i := range d.Modes {
		if d.Modes[i].Hours == nil {
			d.Modes[i].Hours = map[string]int{}
		}
	}
	d.dropOldCatalog()
}

func (d *Document) dropOldCatalog() {
	for id := range d.Apps {
		if OldCatalogSlug(id) {
			delete(d.Apps, id)
		}
	}
	kept := d.Things[:0]
	for _, t := range d.Things {
		if OldCatalogSlug(t.ID) {
			delete(d.Matchers, t.ID)
			continue
		}
		kept = append(kept, t)
	}
	d.Things = kept
	for id := range d.Matchers {
		if OldCatalogSlug(id) {
			delete(d.Matchers, id)
		}
	}
}

func (d Document) validateThings() error {
	seen := map[string]bool{}
	for _, t := range d.Things {
		if t.ID == "" {
			return fmt.Errorf("thing id is required")
		}
		if OldCatalogSlug(t.ID) {
			continue
		}
		if seen[t.ID] {
			return fmt.Errorf("duplicate thing %q", t.ID)
		}
		seen[t.ID] = true
		if strings.TrimSpace(t.Name) == "" {
			return fmt.Errorf("thing %q: name is required", t.ID)
		}
		switch t.Kind {
		case ThingApp, ThingSite:
		default:
			return fmt.Errorf("thing %q: unknown kind %q", t.ID, t.Kind)
		}
	}
	for id := range d.Apps {
		if OldCatalogSlug(id) {
			continue
		}
		if !seen[id] {
			return fmt.Errorf("app %q: unknown thing", id)
		}
	}
	for id := range d.Matchers {
		if OldCatalogSlug(id) {
			continue
		}
		if !seen[id] {
			return fmt.Errorf("matcher %q: unknown thing", id)
		}
	}
	return nil
}

func (d Document) HasPile(id string) bool {
	for _, p := range d.Piles {
		if p.ID == id {
			return true
		}
	}
	return false
}

func (d Document) HasMode(id string) bool {
	for _, m := range d.Modes {
		if m.ID == id {
			return true
		}
	}
	return false
}

func (d Document) ModeKind(id string) (ModeKind, bool) {
	for _, m := range d.Modes {
		if m.ID == id {
			return m.Kind, true
		}
	}
	return "", false
}

func (d Document) SpendBypassesClock(activeID string) bool {
	k, ok := d.ModeKind(activeID)
	return ok && k == KindFreetime
}

func (d Document) HoldsSchedule(activeID string) bool {
	return d.StickyFreetime && d.SpendBypassesClock(activeID)
}

func AdoptPersisted(d *Document) (changed bool) {
	if d.Apps == nil {
		d.Apps = map[string]string{}
	}
	legacy := d.Version < 2
	for id, list := range d.Apps {
		if OldCatalogSlug(id) || list == "minecraft" || list == "youtube" {
			legacy = true
			break
		}
	}
	d.normalize()
	for i := range d.Modes {
		if d.Modes[i].Kind == "" {
			d.Modes[i].Kind = KindAllotment
			changed = true
		}
	}
	n := 0
	for _, m := range d.Modes {
		if m.Kind == KindFreetime {
			n++
		}
	}
	if n == 0 {
		d.Modes = append([]Mode{CanonicalFreetime()}, d.Modes...)
		changed = true
	}
	if legacy {
		d.Things = []Thing{}
		d.Apps = map[string]string{}
		d.Matchers = map[string]Matcher{}
		d.Version = 2
		changed = true
	}
	return changed
}

func (d Document) ModeHours(id, pile string) int {
	for _, m := range d.Modes {
		if m.ID != id {
			continue
		}
		if m.Hours == nil {
			return 0
		}
		return m.Hours[pile]
	}
	return 0
}

func (d Document) EqualPolicy(other Document) bool {
	a := d.Clone()
	b := other.Clone()
	a.Version = 0
	b.Version = 0
	left, err := json.Marshal(a)
	if err != nil {
		return false
	}
	right, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(left) == string(right)
}

func (d Document) MoveApp(appID, listID string) (Document, error) {
	if appID == "" {
		return Document{}, fmt.Errorf("app id is required")
	}
	out := d.Clone()
	if listID == "" {
		kept := out.Things[:0]
		for _, t := range out.Things {
			if t.ID != appID {
				kept = append(kept, t)
			}
		}
		out.Things = kept
		delete(out.Apps, appID)
		delete(out.Matchers, appID)
		return out, nil
	}
	if listID != string(ListFun) && listID != string(ListSchool) {
		return Document{}, fmt.Errorf("app %q: unknown list %q", appID, listID)
	}
	if listID == string(ListFun) && !out.HasPile("fun") {
		return Document{}, fmt.Errorf("app %q: unknown pile %q", appID, listID)
	}
	found := false
	for _, t := range out.Things {
		if t.ID == appID {
			found = true
			break
		}
	}
	if !found {
		return Document{}, fmt.Errorf("app %q: unknown thing", appID)
	}
	out.Apps[appID] = listID
	return out, nil
}

func (d Document) Sitting() []App {
	return nil
}
