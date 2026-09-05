package look

import (
	"fmt"
	"sort"
	"time"
)

type Block struct {
	ID       string `json:"id"`
	Start    int    `json:"start"`
	Duration int    `json:"duration"`
	Mode     string `json:"mode"`
}

func (b Block) End() int {
	return b.Start + b.Duration
}

type Day struct {
	blocks []Block
}

func (b Bedtime) Validate() error {
	if b.LightsOut < 0 || b.LightsOut > 1439 {
		return fmt.Errorf("bedtime lights_out must be 0..1439")
	}
	if b.Duration < 60 || b.Duration > 1440 {
		return fmt.Errorf("bedtime duration must be 60..1440")
	}
	return nil
}

func (b Bedtime) Wake() int {
	return (b.LightsOut + b.Duration) % 1440
}

func (b Bedtime) Contains(minute int) bool {
	if minute < 0 || minute > 1439 {
		return false
	}
	if b.Duration >= 1440 {
		return true
	}
	wake := b.Wake()
	if b.LightsOut < wake {
		return minute >= b.LightsOut && minute < wake
	}
	return minute >= b.LightsOut || minute < wake
}

type span struct{ Start, End int }

func (b Bedtime) spans() []span {
	if b.Duration >= 1440 {
		return []span{{0, 1440}}
	}
	end := b.LightsOut + b.Duration
	if end <= 1440 {
		return []span{{b.LightsOut, end}}
	}
	return []span{{b.LightsOut, 1440}, {0, b.Wake()}}
}

func DayFrom(blocks []Block, bed Bedtime) (Day, error) {
	if err := bed.Validate(); err != nil {
		return Day{}, err
	}
	copied := make([]Block, 0, len(blocks))
	seen := map[string]bool{}
	for i, b := range blocks {
		if b.Mode == "" {
			return Day{}, fmt.Errorf("block %d: mode is required", i)
		}
		if b.Duration < 15 {
			return Day{}, fmt.Errorf("block %q: duration must be >= 15", b.ID)
		}
		if b.Start < 0 || b.Start > 1439 {
			return Day{}, fmt.Errorf("block %q: start must be 0..1439", b.ID)
		}
		if b.Start+b.Duration > 1440 {
			return Day{}, fmt.Errorf("block %q: must not wrap midnight", b.ID)
		}
		id := b.ID
		if id == "" {
			id = fmt.Sprintf("%s-%d", b.Mode, b.Start)
		}
		if seen[id] {
			return Day{}, fmt.Errorf("duplicate block %q", id)
		}
		seen[id] = true
		copied = append(copied, Block{ID: id, Start: b.Start, Duration: b.Duration, Mode: b.Mode})
	}
	sort.Slice(copied, func(i, j int) bool {
		if copied[i].Start == copied[j].Start {
			return copied[i].ID < copied[j].ID
		}
		return copied[i].Start < copied[j].Start
	})
	for i := 1; i < len(copied); i++ {
		if copied[i].Start < copied[i-1].End() {
			return Day{}, fmt.Errorf("blocks %q and %q overlap", copied[i-1].ID, copied[i].ID)
		}
	}
	for _, b := range copied {
		if intersectsBedtime(b, bed) {
			return Day{}, fmt.Errorf("block %q intersects bedtime", b.ID)
		}
	}
	return Day{blocks: copied}, nil
}

func intersectsBedtime(b Block, bed Bedtime) bool {
	for _, s := range bed.spans() {
		if b.Start < s.End && s.Start < b.End() {
			return true
		}
	}
	return false
}

func (d Day) Blocks() []Block {
	out := make([]Block, len(d.blocks))
	copy(out, d.blocks)
	return out
}

func (d Day) Covering(at int) (Block, bool) {
	for _, b := range d.blocks {
		if at >= b.Start && at < b.End() {
			return b, true
		}
	}
	return Block{}, false
}

func (d Day) NextStart(at int) (int, bool) {
	for _, b := range d.blocks {
		if b.Start > at {
			return b.Start, true
		}
	}
	return 0, false
}

func FitBlock(d Day, bed Bedtime, b Block) (Block, error) {
	if b.Mode == "" {
		return Block{}, fmt.Errorf("mode is required")
	}
	start := b.Start
	end := b.End()
	if start < 0 {
		start = 0
	}
	if end > 1440 {
		end = 1440
	}
	for _, other := range d.blocks {
		if other.ID != "" && other.ID == b.ID {
			continue
		}
		if other.End() <= start || other.Start >= end {
			continue
		}
		if other.End() <= b.Start {
			if other.End() > start {
				start = other.End()
			}
			continue
		}
		if other.Start < end {
			end = other.Start
		}
	}
	for _, s := range bed.spans() {
		if end <= s.Start || start >= s.End {
			continue
		}
		if s.End <= b.Start {
			if s.End > start {
				start = s.End
			}
			continue
		}
		if s.Start < end {
			end = s.Start
		}
	}
	if end-start < 15 {
		return Block{}, fmt.Errorf("block does not fit")
	}
	out := Block{ID: b.ID, Start: start, Duration: end - start, Mode: b.Mode}
	day, err := DayFrom(append(withoutBlock(d.blocks, b.ID), out), bed)
	if err != nil {
		return Block{}, err
	}
	got, ok := day.Covering(out.Start)
	if !ok {
		return Block{}, fmt.Errorf("block does not fit")
	}
	return got, nil
}

func withoutBlock(blocks []Block, id string) []Block {
	if id == "" {
		return append([]Block{}, blocks...)
	}
	out := make([]Block, 0, len(blocks))
	for _, b := range blocks {
		if b.ID == id {
			continue
		}
		out = append(out, b)
	}
	return out
}

func DayID(w time.Weekday) string {
	switch w {
	case time.Monday:
		return DayMon
	case time.Tuesday:
		return DayTue
	case time.Wednesday:
		return DayWed
	case time.Thursday:
		return DayThu
	case time.Friday:
		return DayFri
	case time.Saturday:
		return DaySat
	default:
		return DaySun
	}
}

func MinuteOfDay(t time.Time) int {
	return t.Hour()*60 + t.Minute()
}

func (d Document) CoveringAt(now time.Time) (Block, bool) {
	day, err := DayFrom(d.Schedule[DayID(now.Weekday())], d.Bedtime)
	if err != nil {
		return Block{}, false
	}
	return day.Covering(MinuteOfDay(now))
}

func (d Document) OverrideUntil(now time.Time, loc *time.Location) time.Time {
	local := now.In(loc)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	day, err := DayFrom(d.Schedule[DayID(local.Weekday())], d.Bedtime)
	if err != nil {
		return midnight.Add(24 * time.Hour)
	}
	at := MinuteOfDay(local)
	if b, ok := day.Covering(at); ok {
		return midnight.Add(time.Duration(b.End()) * time.Minute)
	}
	if start, ok := day.NextStart(at); ok {
		return midnight.Add(time.Duration(start) * time.Minute)
	}
	return midnight.Add(24 * time.Hour)
}
