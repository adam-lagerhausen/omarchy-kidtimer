package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Policy string

const (
	Bypass  Policy = "bypass"
	Metered Policy = "metered"
)

type Pattern struct {
	Class *regexp.Regexp
	Title *regexp.Regexp
}

type Group struct {
	ID           string
	Policy       Policy
	DailySeconds int
	Overflow     string
	Match        []Pattern
}

type ScheduleWindow struct {
	Days  []time.Weekday
	Start time.Duration
	End   time.Duration
	Mode  string
}

type Lab struct {
	Enabled bool
	Classes []string
}

type Config struct {
	KidName      string
	Location     *time.Location
	Enforcer     bool
	Advertise    bool
	Listen       string
	BedtimeLock  bool
	EmptyLock    bool
	BedtimeStart time.Duration
	BedtimeEnd   time.Duration
	AlwaysOn     []string
	NeverSignal  []string
	Lab          Lab
	RemoteLock   bool
	DefaultMode  string
	Modes        map[string]map[string]int
	Schedule     []ScheduleWindow
	Groups       []Group
	byID         map[string]*Group
}

type fileConfig struct {
	KidName      string                    `toml:"kid_name"`
	Timezone     string                    `toml:"timezone"`
	Enforcer     bool                      `toml:"enforcer"`
	Advertise    bool                      `toml:"advertise"`
	Listen       string                    `toml:"listen"`
	BedtimeLock  bool                      `toml:"bedtime_lock"`
	EmptyLock    bool                      `toml:"empty_lock"`
	BedtimeStart string                    `toml:"bedtime_start"`
	BedtimeEnd   string                    `toml:"bedtime_end"`
	AlwaysOn     []string                  `toml:"always_on"`
	NeverSignal  []string                  `toml:"never_signal"`
	RemoteLock   bool                      `toml:"remote_lock"`
	DefaultMode  string                    `toml:"default_mode"`
	Lab          fileLab                   `toml:"lab"`
	Groups       map[string]fileGroup      `toml:"groups"`
	Modes        map[string]map[string]int `toml:"modes"`
	Schedule     []fileSchedule            `toml:"schedule"`
}

type fileSchedule struct {
	Days  []string `toml:"days"`
	Start string   `toml:"start"`
	End   string   `toml:"end"`
	Mode  string   `toml:"mode"`
}

type fileLab struct {
	Enabled bool     `toml:"enabled"`
	Classes []string `toml:"classes"`
}

type fileGroup struct {
	Policy       string      `toml:"policy"`
	DailySeconds int         `toml:"daily_seconds"`
	Overflow     string      `toml:"overflow"`
	Match        []fileMatch `toml:"match"`
}

type fileMatch struct {
	Class string `toml:"class"`
	Title string `toml:"title"`
}

var groupHeader = regexp.MustCompile(`(?m)^\[groups\.([^.\]]+)\]\s*$`)

func ParseFile(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(raw)
}

func Parse(raw []byte) (*Config, error) {
	var file fileConfig
	if err := toml.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if file.KidName == "" {
		host, err := os.Hostname()
		if err != nil || host == "" {
			return nil, fmt.Errorf("config: kid_name is required")
		}
		file.KidName = host
	}
	if file.Timezone == "" {
		return nil, fmt.Errorf("config: timezone is required")
	}
	loc, err := time.LoadLocation(file.Timezone)
	if err != nil {
		return nil, fmt.Errorf("config: timezone: %w", err)
	}
	start, err := ParseClock(file.BedtimeStart)
	if err != nil {
		return nil, fmt.Errorf("config: bedtime_start: %w", err)
	}
	end, err := ParseClock(file.BedtimeEnd)
	if err != nil {
		return nil, fmt.Errorf("config: bedtime_end: %w", err)
	}

	order := groupOrder(raw)
	if len(order) == 0 {
		return nil, fmt.Errorf("config: no groups")
	}

	cfg := &Config{
		KidName:      file.KidName,
		Location:     loc,
		Enforcer:     file.Enforcer,
		Advertise:    file.Advertise,
		Listen:       file.Listen,
		BedtimeLock:  file.BedtimeLock,
		EmptyLock:    file.EmptyLock,
		BedtimeStart: start,
		BedtimeEnd:   end,
		AlwaysOn:     append([]string{}, file.AlwaysOn...),
		NeverSignal:  append([]string{}, file.NeverSignal...),
		RemoteLock:   file.RemoteLock,
		DefaultMode:  file.DefaultMode,
		Lab: Lab{
			Enabled: file.Lab.Enabled,
			Classes: append([]string{}, file.Lab.Classes...),
		},
		byID: make(map[string]*Group, len(order)),
	}

	seen := map[string]bool{}
	for _, id := range order {
		fg, ok := file.Groups[id]
		if !ok {
			return nil, fmt.Errorf("config: missing table for group %q", id)
		}
		if seen[id] {
			return nil, fmt.Errorf("config: duplicate group %q", id)
		}
		seen[id] = true
		g, err := parseGroup(id, fg)
		if err != nil {
			return nil, err
		}
		cfg.Groups = append(cfg.Groups, g)
		cfg.byID[id] = &cfg.Groups[len(cfg.Groups)-1]
	}

	if err := cfg.validateOverflow(); err != nil {
		return nil, err
	}
	if err := cfg.parseModes(file.Modes); err != nil {
		return nil, err
	}
	if err := cfg.parseSchedule(file.Schedule); err != nil {
		return nil, err
	}
	if cfg.DefaultMode != "" {
		if _, ok := cfg.Modes[cfg.DefaultMode]; !ok {
			return nil, fmt.Errorf("config: default_mode %q is not a mode", cfg.DefaultMode)
		}
	}
	return cfg, nil
}

func parseGroup(id string, fg fileGroup) (Group, error) {
	g := Group{
		ID:           id,
		DailySeconds: fg.DailySeconds,
		Overflow:     fg.Overflow,
	}
	switch Policy(fg.Policy) {
	case Bypass, Metered:
		g.Policy = Policy(fg.Policy)
	default:
		return Group{}, fmt.Errorf("config: group %q: policy must be bypass or metered", id)
	}
	if g.Policy == Bypass && g.Overflow != "" {
		return Group{}, fmt.Errorf("config: group %q: bypass cannot set overflow", id)
	}
	if len(fg.Match) == 0 {
		return g, nil
	}
	for i, m := range fg.Match {
		pat, err := compileMatch(id, i, m)
		if err != nil {
			return Group{}, err
		}
		g.Match = append(g.Match, pat)
	}
	return g, nil
}

func compileMatch(id string, i int, m fileMatch) (Pattern, error) {
	if m.Class == "" && m.Title == "" {
		return Pattern{}, fmt.Errorf("config: group %q match %d: class or title is required", id, i)
	}
	if isAnyClass(m.Class) && m.Title != "" {
		return Pattern{}, fmt.Errorf("config: group %q match %d: class = \".*\" with a title regex is not allowed", id, i)
	}
	var p Pattern
	if m.Class != "" {
		re, err := regexp.Compile(m.Class)
		if err != nil {
			return Pattern{}, fmt.Errorf("config: group %q match %d class: %w", id, i, err)
		}
		p.Class = re
	}
	if m.Title != "" {
		re, err := regexp.Compile(m.Title)
		if err != nil {
			return Pattern{}, fmt.Errorf("config: group %q match %d title: %w", id, i, err)
		}
		p.Title = re
	}
	return p, nil
}

func isAnyClass(class string) bool {
	s := strings.TrimSpace(class)
	return s == ".*" || s == "(?i).*" || s == "^(.*)$"
}

var weekdayName = map[string]time.Weekday{
	"sun": time.Sunday,
	"mon": time.Monday,
	"tue": time.Tuesday,
	"wed": time.Wednesday,
	"thu": time.Thursday,
	"fri": time.Friday,
	"sat": time.Saturday,
}

func (c *Config) parseModes(raw map[string]map[string]int) error {
	if len(raw) == 0 {
		c.Modes = map[string]map[string]int{}
		return nil
	}
	c.Modes = make(map[string]map[string]int, len(raw))
	for id, groups := range raw {
		allot := make(map[string]int, len(groups))
		for group, seconds := range groups {
			g, ok := c.byID[group]
			if !ok {
				return fmt.Errorf("config: mode %q names missing group %q", id, group)
			}
			if g.Policy != Metered {
				return fmt.Errorf("config: mode %q names bypass group %q", id, group)
			}
			if seconds < 0 {
				return fmt.Errorf("config: mode %q group %q: daily_seconds must be >= 0", id, group)
			}
			allot[group] = seconds
		}
		c.Modes[id] = allot
	}
	return nil
}

func (c *Config) parseSchedule(rows []fileSchedule) error {
	for i, row := range rows {
		if row.Mode == "" {
			return fmt.Errorf("config: schedule %d: mode is required", i)
		}
		if _, ok := c.Modes[row.Mode]; !ok {
			return fmt.Errorf("config: schedule %d: mode %q is not a mode", i, row.Mode)
		}
		if len(row.Days) == 0 {
			return fmt.Errorf("config: schedule %d: days is required", i)
		}
		w := ScheduleWindow{Mode: row.Mode}
		seen := map[time.Weekday]bool{}
		for _, name := range row.Days {
			d, ok := weekdayName[strings.ToLower(name)]
			if !ok {
				return fmt.Errorf("config: schedule %d: unknown day %q", i, name)
			}
			if seen[d] {
				continue
			}
			seen[d] = true
			w.Days = append(w.Days, d)
		}
		start, err := ParseClock(row.Start)
		if err != nil {
			return fmt.Errorf("config: schedule %d start: %w", i, err)
		}
		end, err := ParseClock(row.End)
		if err != nil {
			return fmt.Errorf("config: schedule %d end: %w", i, err)
		}
		w.Start = start
		w.End = end
		c.Schedule = append(c.Schedule, w)
	}
	return nil
}

func (c *Config) validateOverflow() error {
	for i := range c.Groups {
		g := &c.Groups[i]
		if g.Overflow == "" {
			continue
		}
		if g.Overflow == g.ID {
			return fmt.Errorf("config: group %q: overflow cannot point at itself", g.ID)
		}
		target, ok := c.byID[g.Overflow]
		if !ok {
			return fmt.Errorf("config: group %q: overflow %q is not a group", g.ID, g.Overflow)
		}
		if target.Policy != Metered {
			return fmt.Errorf("config: group %q: overflow %q is not metered", g.ID, g.Overflow)
		}
		if target.Overflow != "" {
			return fmt.Errorf("config: group %q: overflow %q already has overflow set", g.ID, g.Overflow)
		}
	}
	return nil
}

func groupOrder(raw []byte) []string {
	matches := groupHeader.FindAllSubmatch(raw, -1)
	var ids []string
	seen := map[string]bool{}
	for _, m := range matches {
		id := string(m[1])
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	return ids
}

func ParseClock(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}
	var h, m int
	n, err := fmt.Sscanf(s, "%d:%d", &h, &m)
	if err != nil || n != 2 || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("want HH:MM")
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute, nil
}

func FormatClock(d time.Duration) string {
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	return fmt.Sprintf("%02d:%02d", h, m)
}

func clockElapsed(now time.Time) time.Duration {
	return time.Duration(now.Hour())*time.Hour + time.Duration(now.Minute())*time.Minute + time.Duration(now.Second())*time.Second
}

func ClockContains(loc *time.Location, start, end time.Duration, now time.Time) bool {
	local := now.In(loc)
	elapsed := clockElapsed(local)
	if start == end {
		return true
	}
	if start < end {
		return elapsed >= start && elapsed < end
	}
	return elapsed >= start || elapsed < end
}

func (c *Config) Group(id string) (*Group, bool) {
	g, ok := c.byID[id]
	return g, ok
}

func (c *Config) HasGroup(id string) bool {
	_, ok := c.byID[id]
	return ok
}

func (c *Config) AlwaysOnClass(class string) bool {
	if class == "" {
		return true
	}
	for _, name := range c.AlwaysOn {
		if name == class {
			return true
		}
	}
	return false
}

func (c *Config) NeverSignalName(name string) bool {
	for _, n := range c.NeverSignal {
		if n == name {
			return true
		}
	}
	return false
}

func (c *Config) LabClass(class string) bool {
	for _, name := range c.Lab.Classes {
		if name == class {
			return true
		}
	}
	return false
}

func (c *Config) Match(class, title string) (string, bool) {
	for i := range c.Groups {
		g := &c.Groups[i]
		for _, p := range g.Match {
			if patternHit(p, class, title) {
				return g.ID, true
			}
		}
	}
	return "", false
}

func patternHit(p Pattern, class, title string) bool {
	if p.Class != nil && !p.Class.MatchString(class) {
		return false
	}
	if p.Title != nil && !p.Title.MatchString(title) {
		return false
	}
	return true
}

func (c *Config) BedtimeContains(now time.Time) bool {
	return ClockContains(c.Location, c.BedtimeStart, c.BedtimeEnd, now)
}

func SecondsUntil(loc *time.Location, start time.Duration, now time.Time) int {
	local := now.In(loc)
	begin := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).Add(start)
	if !local.Before(begin) {
		begin = begin.Add(24 * time.Hour)
	}
	n := int(begin.Sub(local) / time.Second)
	if n < 0 {
		return 0
	}
	return n
}

func (c *Config) SecondsUntilBedtime(now time.Time) int {
	if c.BedtimeContains(now) {
		return 0
	}
	return SecondsUntil(c.Location, c.BedtimeStart, now)
}

func (c *Config) MatchingWindow(now time.Time) (ScheduleWindow, bool) {
	for _, w := range c.Schedule {
		if w.Contains(now, c.Location) {
			return w, true
		}
	}
	return ScheduleWindow{}, false
}

func (w ScheduleWindow) Contains(now time.Time, loc *time.Location) bool {
	local := now.In(loc)
	elapsed := clockElapsed(local)
	if w.Start < w.End {
		return w.hasDay(local.Weekday()) && elapsed >= w.Start && elapsed < w.End
	}
	if w.Start == w.End {
		return w.hasDay(local.Weekday())
	}
	if elapsed >= w.Start && w.hasDay(local.Weekday()) {
		return true
	}
	// After midnight the calendar day changed, so match yesterday's listed day.
	if elapsed < w.End && w.hasDay(priorWeekday(local.Weekday())) {
		return true
	}
	return false
}

func (w ScheduleWindow) hasDay(d time.Weekday) bool {
	for _, day := range w.Days {
		if day == d {
			return true
		}
	}
	return false
}

func (w ScheduleWindow) EndTime(now time.Time, loc *time.Location) time.Time {
	local := now.In(loc)
	end := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc).Add(w.End)
	if w.Start < w.End {
		return end
	}
	if clockElapsed(local) >= w.Start {
		return end.Add(24 * time.Hour)
	}
	return end
}

func priorWeekday(d time.Weekday) time.Weekday {
	if d == time.Sunday {
		return time.Saturday
	}
	return d - 1
}

func (c *Config) DefaultCreditGroup() string {
	if c.HasGroup("fun") {
		return "fun"
	}
	return ""
}
