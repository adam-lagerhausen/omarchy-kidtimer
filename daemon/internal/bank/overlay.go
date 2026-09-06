package bank

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/look"
)

const (
	metaParentLock     = "parent_lock"
	metaActiveMode     = "active_mode"
	metaOverrideUntil  = "mode_override_until"
	metaBedtimeStart   = "bedtime_start"
	metaBedtimeEnd     = "bedtime_end"
	metaBedtimeLock    = "bedtime_lock"
	metaModeMinutes    = "mode_minutes"
	metaLook           = "look"
	metaPaired         = "paired"
	metaParentPin      = "parent_pin"
	metaBedtimeHold    = "bedtime_hold_until"
	metaRefillDeferred = "refill_deferred"
)

type overlay struct {
	parentLock    bool
	activeMode    string
	overrideUntil time.Time
	bedtimeStart  *time.Duration
	bedtimeEnd    *time.Duration
	bedtimeLock   *bool
	modeMinutes   map[string]map[string]int
	parentPin     string
	holdUntil     time.Time
}

func (b *Bank) loadOverlayLocked() error {
	b.ov.modeMinutes = map[string]map[string]int{}
	v, ok, err := b.metaGet(metaParentLock)
	if err != nil {
		return err
	}
	b.ov.parentLock = ok && v == "true"
	v, ok, err = b.metaGet(metaActiveMode)
	if err != nil {
		return err
	}
	if ok {
		b.ov.activeMode = v
	}
	v, ok, err = b.metaGet(metaOverrideUntil)
	if err != nil {
		return err
	}
	if ok && v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return fmt.Errorf("overlay mode_override_until: %w", err)
		}
		b.ov.overrideUntil = t
	}
	if err := b.loadClockMeta(metaBedtimeStart, &b.ov.bedtimeStart); err != nil {
		return err
	}
	if err := b.loadClockMeta(metaBedtimeEnd, &b.ov.bedtimeEnd); err != nil {
		return err
	}
	v, ok, err = b.metaGet(metaBedtimeLock)
	if err != nil {
		return err
	}
	if ok {
		on := v == "true"
		b.ov.bedtimeLock = &on
	}
	v, ok, err = b.metaGet(metaModeMinutes)
	if err != nil {
		return err
	}
	if ok && v != "" {
		if err := json.Unmarshal([]byte(v), &b.ov.modeMinutes); err != nil {
			return fmt.Errorf("overlay mode_minutes: %w", err)
		}
		if b.ov.modeMinutes == nil {
			b.ov.modeMinutes = map[string]map[string]int{}
		}
	}
	v, ok, err = b.metaGet(metaParentPin)
	if err != nil {
		return err
	}
	if ok {
		b.ov.parentPin = v
	}
	v, ok, err = b.metaGet(metaBedtimeHold)
	if err != nil {
		return err
	}
	if ok && v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return fmt.Errorf("overlay bedtime_hold_until: %w", err)
		}
		b.ov.holdUntil = t
	}
	return nil
}

func (b *Bank) loadClockMeta(key string, dst **time.Duration) error {
	v, ok, err := b.metaGet(key)
	if err != nil {
		return err
	}
	if !ok || v == "" {
		return nil
	}
	d, err := config.ParseClock(v)
	if err != nil {
		return fmt.Errorf("overlay %s: %w", key, err)
	}
	*dst = &d
	return nil
}

func (b *Bank) metaGet(key string) (string, bool, error) {
	var v string
	err := b.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}

func (b *Bank) metaSet(key, value string) error {
	_, err := b.db.Exec(
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

func (b *Bank) ApplyMode(actor *Token, id string) error {
	return b.ApplyModeSpec(actor, ModeApply{ID: id, Kind: UntilDefault})
}

func (b *Bank) ApplyModeSpec(actor *Token, spec ModeApply) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return err
	}
	if err := requireParent(actor); err != nil {
		return err
	}
	return b.applyModeSpecLocked(spec)
}

func (b *Bank) applyModeSpecLocked(spec ModeApply) error {
	switch spec.Kind {
	case UntilResume:
		if err := b.setStickyLocked(false); err != nil {
			return err
		}
		b.ov.overrideUntil = time.Time{}
		if err := b.metaSet(metaOverrideUntil, ""); err != nil {
			return err
		}
		b.ov.activeMode = ""
		return b.metaSet(metaActiveMode, "")
	case UntilSticky:
		id := spec.ID
		if id == "" {
			id = look.FreetimeID
		}
		if id != look.FreetimeID {
			return fmt.Errorf("%w: sticky only for freetime", ErrInvalid)
		}
		if err := b.setStickyLocked(true); err != nil {
			return err
		}
		return b.applyModeLocked(id, true, false)
	case UntilFor:
		if spec.ID == "" {
			return fmt.Errorf("%w: id is required", ErrInvalid)
		}
		switch spec.Minutes {
		case 15, 30, 60, 120:
		default:
			return fmt.Errorf("%w: minutes must be 15, 30, 60, or 120", ErrInvalid)
		}
		if err := b.setStickyLocked(false); err != nil {
			return err
		}
		if err := b.applyModeLocked(spec.ID, true, false); err != nil {
			return err
		}
		until := b.now().Add(time.Duration(spec.Minutes) * time.Minute)
		b.ov.overrideUntil = until
		return b.metaSet(metaOverrideUntil, until.UTC().Format(time.RFC3339))
	default:
		if spec.ID == "" {
			return fmt.Errorf("%w: id is required", ErrInvalid)
		}
		if spec.Kind == UntilNext || spec.ID != look.FreetimeID {
			if err := b.setStickyLocked(false); err != nil {
				return err
			}
		}
		return b.applyModeLocked(spec.ID, true, true)
	}
}

func (b *Bank) setStickyLocked(v bool) error {
	if b.look.StickyFreetime == v {
		return nil
	}
	doc := b.look.Clone()
	doc.StickyFreetime = v
	if err := doc.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	doc.Version = b.look.Version + 1
	b.look = doc
	return b.persistLookLocked()
}

func (b *Bank) SetParentLock(actor *Token, locked bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := requireParent(actor); err != nil {
		return err
	}
	if b.ov.parentLock == locked {
		return nil
	}
	b.ov.parentLock = locked
	return b.metaSet(metaParentLock, boolMeta(locked))
}

func (b *Bank) SetBedtime(actor *Token, start, end string, lock *bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := requireParent(actor); err != nil {
		return err
	}
	bed := b.look.Bedtime
	lights := bed.LightsOut
	wake := bed.Wake()
	clocks := false
	if start != "" {
		d, err := config.ParseClock(start)
		if err != nil {
			return fmt.Errorf("%w: bedtime_start: %w", ErrInvalid, err)
		}
		lights = int(d / time.Minute)
		b.ov.bedtimeStart = &d
		if err := b.metaSet(metaBedtimeStart, config.FormatClock(d)); err != nil {
			return err
		}
		clocks = true
	}
	if end != "" {
		d, err := config.ParseClock(end)
		if err != nil {
			return fmt.Errorf("%w: bedtime_end: %w", ErrInvalid, err)
		}
		wake = int(d / time.Minute)
		b.ov.bedtimeEnd = &d
		if err := b.metaSet(metaBedtimeEnd, config.FormatClock(d)); err != nil {
			return err
		}
		clocks = true
	}
	if clocks {
		dur := wake - lights
		if dur <= 0 {
			dur += 1440
		}
		if dur < 60 {
			return fmt.Errorf("%w: bedtime duration must be >= 60", ErrInvalid)
		}
		doc := b.look.Clone()
		doc.Bedtime = look.Bedtime{LightsOut: lights, Duration: dur}
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalid, err)
		}
		doc.Version = b.look.Version + 1
		b.look = doc
		if err := b.persistLookLocked(); err != nil {
			return err
		}
	}
	if lock != nil {
		v := *lock
		b.ov.bedtimeLock = &v
		if err := b.metaSet(metaBedtimeLock, boolMeta(v)); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bank) SetModeMinutes(actor *Token, modeID, group string, seconds int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := requireParent(actor); err != nil {
		return err
	}
	if !b.look.HasMode(modeID) {
		return fmt.Errorf("%w: unknown mode %q", ErrInvalid, modeID)
	}
	if b.look.SpendBypassesClock(modeID) {
		return fmt.Errorf("%w: freetime has no hours", ErrInvalid)
	}
	if !b.look.HasPile(group) {
		return fmt.Errorf("%w: unknown group %q", ErrInvalid, group)
	}
	if seconds < 0 {
		return fmt.Errorf("%w: seconds must be >= 0", ErrInvalid)
	}
	doc := b.look.Clone()
	for i := range doc.Modes {
		if doc.Modes[i].ID != modeID {
			continue
		}
		if doc.Modes[i].Hours == nil {
			doc.Modes[i].Hours = map[string]int{}
		}
		doc.Modes[i].Hours[group] = seconds
		break
	}
	if err := doc.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	doc.Version = b.look.Version + 1
	b.look = doc
	if err := b.persistLookLocked(); err != nil {
		return err
	}
	if b.ov.modeMinutes[modeID] == nil {
		b.ov.modeMinutes[modeID] = map[string]int{}
	}
	b.ov.modeMinutes[modeID][group] = seconds
	raw, err := json.Marshal(b.ov.modeMinutes)
	if err != nil {
		return err
	}
	return b.metaSet(metaModeMinutes, string(raw))
}

func (b *Bank) Look(actor *Token) (look.Document, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := requireParent(actor); err != nil {
		return look.Document{}, err
	}
	return b.look.Clone(), nil
}

func (b *Bank) PutLook(actor *Token, doc look.Document) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := requireParent(actor); err != nil {
		return err
	}
	incoming := doc.Clone()
	if err := incoming.Validate(); err != nil {
		return fmt.Errorf("%w: %s", ErrInvalid, err)
	}
	if incoming.EqualPolicy(b.look) {
		return nil
	}
	incoming.Version = b.look.Version + 1
	b.look = incoming
	if err := b.persistLookLocked(); err != nil {
		return err
	}
	if b.ov.activeMode != "" && !b.look.HasMode(b.ov.activeMode) {
		b.ov.activeMode = ""
		if err := b.metaSet(metaActiveMode, ""); err != nil {
			return err
		}
	}
	return b.ensureBalancesLocked(b.day())
}

func (b *Bank) SyncSchedule() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.resetDayLocked(); err != nil {
		return err
	}
	return b.syncScheduleLocked()
}

func (b *Bank) ParentLocked() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ov.parentLock
}

func (b *Bank) ActiveMode() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.ov.activeMode
}

func (b *Bank) BedtimeLockEnabled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.effectiveBedtimeLockLocked()
}

func (b *Bank) applyModeLocked(id string, refill, setOverride bool) error {
	if !b.look.HasMode(id) {
		return fmt.Errorf("%w: unknown mode %q", ErrInvalid, id)
	}
	b.ov.activeMode = id
	if err := b.metaSet(metaActiveMode, id); err != nil {
		return err
	}
	if setOverride {
		return b.setOverrideLocked()
	}
	b.ov.overrideUntil = time.Time{}
	return b.metaSet(metaOverrideUntil, "")
}

func (b *Bank) setOverrideLocked() error {
	until := b.look.OverrideUntil(b.now(), b.cfg.Location)
	b.ov.overrideUntil = until
	return b.metaSet(metaOverrideUntil, until.UTC().Format(time.RFC3339))
}

func (b *Bank) syncScheduleLocked() error {
	return nil
}

func (b *Bank) effectiveModesLocked() map[string]map[string]int {
	out := make(map[string]map[string]int, len(b.look.Modes))
	for _, m := range b.look.Modes {
		hours := make(map[string]int, len(m.Hours))
		for g, sec := range m.Hours {
			hours[g] = sec
		}
		out[m.ID] = hours
	}
	return out
}

func (b *Bank) effectiveBedtimeLocked() (start, end time.Duration) {
	bed := b.look.Bedtime
	start = time.Duration(bed.LightsOut) * time.Minute
	end = time.Duration(bed.Wake()) * time.Minute
	return start, end
}

func (b *Bank) effectiveBedtimeLockLocked() bool {
	if b.ov.bedtimeLock != nil {
		return *b.ov.bedtimeLock
	}
	return b.cfg.BedtimeLock
}

func (b *Bank) allotmentLocked(id string) int {
	if g, ok := b.cfg.Group(id); ok && g.Policy == config.Bypass {
		return 0
	}
	if id == "fun" && len(b.look.FunHours) != 0 {
		key := strings.ToLower(b.now().In(b.cfg.Location).Weekday().String()[:3])
		if sec, ok := b.look.FunHours[key]; ok {
			return sec
		}
	}
	if sec, ok := b.look.PileHours[id]; ok {
		return sec
	}
	if g, ok := b.cfg.Group(id); ok {
		return g.DailySeconds
	}
	return 0
}

func (b *Bank) ensureLookLocked() error {
	v, ok, err := b.metaGet(metaLook)
	if err != nil {
		return err
	}
	if ok && v != "" {
		var doc look.Document
		if err := json.Unmarshal([]byte(v), &doc); err != nil {
			return fmt.Errorf("look: %w", err)
		}
		doc = doc.Clone()
		changed := look.AdoptPersisted(&doc)
		if err := doc.Validate(); err != nil {
			return fmt.Errorf("look: %w", err)
		}
		b.look = doc
		if changed {
			return b.persistLookLocked()
		}
		return nil
	}
	doc, err := look.Seed(b.cfg)
	if err != nil {
		return err
	}
	b.look = doc
	return b.persistLookLocked()
}

func (b *Bank) persistLookLocked() error {
	raw, err := json.Marshal(b.look)
	if err != nil {
		return err
	}
	return b.metaSet(metaLook, string(raw))
}

func (b *Bank) writeRemainingLocked(group string, remaining int, day string) error {
	_, err := b.db.Exec(
		`INSERT INTO balances (group_id, remaining, day) VALUES (?, ?, ?)
		 ON CONFLICT(group_id) DO UPDATE SET remaining = excluded.remaining, day = excluded.day`,
		group, remaining, day,
	)
	return err
}

func requireParent(actor *Token) error {
	if actor == nil {
		return ErrUnauthorized
	}
	if actor.Kind != KindParent {
		return ErrForbidden
	}
	return nil
}

func boolMeta(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
