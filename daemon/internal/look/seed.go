package look

import (
	"fmt"
	"sort"
	"time"

	"kidtimer/daemon/internal/config"
)

var preferredModes = []string{"morning", "homework", "evening"}

func Seed(cfg *config.Config) (Document, error) {
	if cfg == nil {
		return Document{}, fmt.Errorf("config is required")
	}
	pilesSet, err := PilesFrom([]Pile{{ID: string(ListFun), Name: "Fun"}}, nil)
	if err != nil {
		return Document{}, err
	}
	allotment := seedModes(cfg, pilesSet.List())
	modes := append([]Mode{CanonicalFreetime()}, allotment...)
	schedule, err := seedSchedule(cfg, modes)
	if err != nil {
		return Document{}, err
	}
	funSeconds := 3600
	if g, ok := cfg.Group("fun"); ok && g.DailySeconds > 0 {
		funSeconds = g.DailySeconds
	}
	doc := Document{
		Version:  2,
		Piles:    pilesSet.List(),
		Things:   []Thing{},
		Apps:     map[string]string{},
		Matchers: map[string]Matcher{},
		PileHours: map[string]int{
			string(ListFun): funSeconds,
		},
		FunHours: map[string]int{
			DayMon: 3600,
			DayTue: 3600,
			DayWed: 3600,
			DayThu: 3600,
			DayFri: 3600,
			DaySat: 7200,
			DaySun: 7200,
		},
		Modes:          modes,
		Schedule:       schedule,
		Bedtime:        bedtimeFromConfig(cfg),
		StickyFreetime: false,
	}
	doc.normalize()
	if err := doc.Validate(); err != nil {
		return Document{}, err
	}
	return doc, nil
}

func seedModes(cfg *config.Config, piles []Pile) []Mode {
	seen := map[string]bool{}
	var ids []string
	for _, id := range preferredModes {
		if _, ok := cfg.Modes[id]; ok {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	var extra []string
	for id := range cfg.Modes {
		if seen[id] {
			continue
		}
		extra = append(extra, id)
	}
	sort.Strings(extra)
	ids = append(ids, extra...)
	out := make([]Mode, 0, len(ids))
	for _, id := range ids {
		hours := map[string]int{}
		src := cfg.Modes[id]
		for _, p := range piles {
			hours[p.ID] = src[p.ID]
		}
		out = append(out, Mode{ID: id, Name: id, Kind: KindAllotment, Hours: hours})
	}
	return out
}

func seedSchedule(cfg *config.Config, modes []Mode) (map[string][]Block, error) {
	out := map[string][]Block{}
	for _, id := range DayIDs {
		out[id] = []Block{}
	}
	modeOK := map[string]bool{}
	for _, m := range modes {
		modeOK[m.ID] = true
	}
	for _, w := range cfg.Schedule {
		if !modeOK[w.Mode] {
			continue
		}
		start := int(w.Start / time.Minute)
		end := int(w.End / time.Minute)
		if end <= start {
			continue
		}
		dur := end - start
		if dur < 15 {
			continue
		}
		for _, day := range w.Days {
			id := DayID(day)
			block := Block{
				ID:       fmt.Sprintf("%s-%s-%d", id, w.Mode, start),
				Start:    start,
				Duration: dur,
				Mode:     w.Mode,
			}
			placed, err := placeWithoutOverlap(out[id], block, bedtimeFromConfig(cfg))
			if err != nil {
				continue
			}
			out[id] = placed
		}
	}
	return out, nil
}

func placeWithoutOverlap(existing []Block, b Block, bed Bedtime) ([]Block, error) {
	clipped := b
	for _, other := range existing {
		if other.End() <= clipped.Start || other.Start >= clipped.End() {
			continue
		}
		if other.Start <= clipped.Start {
			clipped.Start = other.End()
			clipped.Duration = b.End() - clipped.Start
		} else {
			clipped.Duration = other.Start - clipped.Start
		}
		if clipped.Duration < 15 {
			return nil, fmt.Errorf("trimmed away")
		}
	}
	day, err := DayFrom(append(append([]Block{}, existing...), clipped), bed)
	if err != nil {
		return nil, err
	}
	return day.Blocks(), nil
}

func bedtimeFromConfig(cfg *config.Config) Bedtime {
	lights := int(cfg.BedtimeStart / time.Minute)
	wake := int(cfg.BedtimeEnd / time.Minute)
	dur := wake - lights
	if dur <= 0 {
		dur += 1440
	}
	if dur < 60 {
		dur = 60
	}
	if dur > 1440 {
		dur = 1440
	}
	return Bedtime{LightsOut: lights, Duration: dur}
}
