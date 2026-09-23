package bank

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/look"
	"kidtimer/daemon/internal/pin"
)

const (
	pinFailLimit = 5
	pinCoolFor   = 30 * time.Second
)

func (b *Bank) SetParentPIN(actor *Token, digits, encoded string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := requireParent(actor); err != nil {
		return err
	}
	hash := encoded
	if digits != "" {
		h, err := pin.Hash(digits)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrInvalid, err)
		}
		hash = h
	}
	if !pin.ValidEncoded(hash) {
		return fmt.Errorf("%w: pin hash", ErrInvalid)
	}
	b.ov.parentPin = hash
	b.pinFails = 0
	b.pinCool = time.Time{}
	return b.metaSet(metaParentPin, hash)
}

func (b *Bank) PinApprove(actor *Token, digits, askID string) (*Grant, *Ask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return nil, nil, err
	}
	if err := requirePinActor(actor); err != nil {
		return nil, nil, err
	}
	if err := b.checkPinLocked(digits); err != nil {
		return nil, nil, err
	}
	return b.decideLocked(actor, askID, "approve")
}

func (b *Bank) PinGrant(actor *Token, digits string, seconds int) (*Grant, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return nil, err
	}
	if err := requirePinActor(actor); err != nil {
		return nil, err
	}
	if seconds <= 0 {
		return nil, fmt.Errorf("%w: seconds must be positive", ErrInvalid)
	}
	if seconds > MaxAskSeconds {
		return nil, fmt.Errorf("%w: seconds exceeds %d", ErrInvalid, MaxAskSeconds)
	}
	if err := b.checkPinLocked(digits); err != nil {
		return nil, err
	}
	if !b.knownClockLocked("fun") {
		return nil, fmt.Errorf("%w: unknown group %q", ErrInvalid, "fun")
	}
	idem := fmt.Sprintf("parent-pin:%s:%d", actor.ID, b.now().UnixNano())
	bodyHash := grantBodyHash("fun", seconds, "parent-pin")
	var g *Grant
	var err error
	if b.bedtimeStayUpLocked() {
		g, err = b.grantStayUpLocked(actor, "fun", seconds, "parent-pin", idem, "parent-pin", bodyHash)
	} else {
		g, err = b.postLocked(actor, "fun", seconds, "parent-pin", idem, "parent-pin", bodyHash)
	}
	if err != nil {
		return nil, err
	}
	if err := b.dismissPendingAsksLocked(); err != nil {
		return nil, err
	}
	if err := b.clearParentLockLocked(); err != nil {
		return nil, err
	}
	if err := b.liftBreakLocked(); err != nil {
		return nil, err
	}
	return g, nil
}

func (b *Bank) PinEndBreak(actor *Token, digits string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return err
	}
	if err := requirePinActor(actor); err != nil {
		return err
	}
	if err := b.checkPinLocked(digits); err != nil {
		return err
	}
	return b.liftBreakLocked()
}

func (b *Bank) closeSittingLocked() error {
	var id int64
	var updated int64
	err := b.db.QueryRow(
		`SELECT id, updated_unix FROM today_spans WHERE day = ? ORDER BY id DESC LIMIT 1`,
		b.day(),
	).Scan(&id, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if id > b.ov.playCut {
		b.ov.playCut = id
		if err := b.metaSet(metaPlaySittingCut, strconv.FormatInt(id, 10)); err != nil {
			return err
		}
	}
	if b.now().Unix()-updated > todayGapSeconds {
		return nil
	}
	stale := b.now().Unix() - int64(todayGapSeconds) - 1
	_, err = b.db.Exec(`UPDATE today_spans SET updated_unix = ? WHERE id = ?`, stale, id)
	return err
}

func (b *Bank) OverlayActive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.overlayActiveLocked()
}

func (b *Bank) SyncSaveCover() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return err
	}
	return b.syncSaveCoverLocked()
}

func (b *Bank) overlayActiveLocked() bool {
	if b.ov.parentPin == "" {
		return false
	}
	if b.breakActiveLocked() {
		return true
	}
	if b.effectiveBedtimeLockLocked() && b.bedtimeActiveLocked() && !b.stayUpActiveLocked() {
		return true
	}
	if b.cfg.RemoteLock && b.ov.parentLock {
		return true
	}
	if b.cfg.EmptyLock && !b.cfg.Lab.Enabled {
		left, err := b.remainingLocked("fun")
		if err == nil && left <= 0 {
			return true
		}
	}
	return false
}

func (b *Bank) saveCoverEligibleLocked() bool {
	if !b.overlayActiveLocked() {
		return false
	}
	if b.cfg.RemoteLock && b.ov.parentLock {
		return false
	}
	if b.breakActiveLocked() {
		return false
	}
	return true
}

func (b *Bank) syncSaveCoverLocked() error {
	if !b.overlayActiveLocked() {
		return b.clearSaveCoverLocked()
	}
	if !b.saveCoverEligibleLocked() {
		return nil
	}
	if !b.ov.saveUntil.IsZero() {
		return nil
	}
	until := b.now().Add(saveCoverDuration)
	b.ov.saveUntil = until
	return b.metaSet(metaSaveCoverUntil, until.UTC().Format(time.RFC3339))
}

func (b *Bank) clearSaveCoverLocked() error {
	if b.ov.saveUntil.IsZero() {
		return nil
	}
	b.ov.saveUntil = time.Time{}
	return b.metaSet(metaSaveCoverUntil, "")
}

func (b *Bank) saveSecondsLocked() int {
	if !b.saveCoverEligibleLocked() {
		return 0
	}
	if b.ov.saveUntil.IsZero() {
		return 0
	}
	d := b.ov.saveUntil.Sub(b.now())
	if d <= 0 {
		return 0
	}
	n := int((d + time.Second - 1) / time.Second)
	if n > int(saveCoverDuration/time.Second) {
		n = int(saveCoverDuration / time.Second)
	}
	return n
}

func (b *Bank) SyncPlayBreak() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return err
	}
	return b.syncPlayBreakLocked()
}

func (b *Bank) syncPlayBreakLocked() error {
	if b.ov.parentPin == "" {
		return nil
	}
	now := b.now()
	if !b.ov.breakUntil.IsZero() && !now.Before(b.ov.breakUntil) {
		if err := b.clearBreakLocked(); err != nil {
			return err
		}
	}
	if b.breakActiveLocked() {
		return nil
	}
	left, err := b.remainingLocked("fun")
	if err == nil && left <= 0 {
		return nil
	}
	play := b.look.PlayMinutes
	if play <= 0 {
		play = look.DefaultPlayMinutes
	}
	if b.sittingSecondsLocked() < play*60 {
		return nil
	}
	brk := b.look.BreakMinutes
	if brk <= 0 {
		brk = look.DefaultBreakMinutes
	}
	until := now.Add(time.Duration(brk) * time.Minute)
	b.ov.breakUntil = until
	if err := b.metaSet(metaBreakUntil, until.UTC().Format(time.RFC3339)); err != nil {
		return err
	}
	return b.closeSittingLocked()
}

func (b *Bank) clearBreakLocked() error {
	if b.ov.breakUntil.IsZero() {
		return nil
	}
	b.ov.breakUntil = time.Time{}
	return b.metaSet(metaBreakUntil, "")
}

func (b *Bank) liftBreakLocked() error {
	if !b.breakActiveLocked() {
		return nil
	}
	if err := b.clearBreakLocked(); err != nil {
		return err
	}
	return b.closeSittingLocked()
}

func (b *Bank) breakActiveLocked() bool {
	if b.ov.breakUntil.IsZero() {
		return false
	}
	return b.now().Before(b.ov.breakUntil)
}

func (b *Bank) breakSecondsLocked() int {
	if !b.breakActiveLocked() {
		return 0
	}
	d := b.ov.breakUntil.Sub(b.now())
	if d <= 0 {
		return 0
	}
	n := int((d + time.Second - 1) / time.Second)
	// Cap a clock jump at the longest break a parent can set. Using the
	// current BREAK setting here freezes the countdown if they shorten it
	// while this break is already running.
	max := look.MaxBreakMinutes * 60
	if n > max {
		n = max
	}
	return n
}

type playSpan struct {
	id      int64
	start   int64
	updated int64
	seconds int
}

func (b *Bank) sittingSecondsLocked() int {
	day := b.day()
	rows, err := b.db.Query(
		`SELECT id, start_unix, updated_unix, start_min, seconds FROM today_spans WHERE day = ? AND id > ? ORDER BY id`,
		day, b.ov.playCut,
	)
	if err != nil {
		return 0
	}
	defer rows.Close()
	var spans []playSpan
	for rows.Next() {
		var row playSpan
		var startMin int
		if err := rows.Scan(&row.id, &row.start, &row.updated, &startMin, &row.seconds); err != nil {
			return 0
		}
		row.start = b.spanStartUnix(day, startMin, row.start)
		spans = append(spans, row)
	}
	if err := rows.Err(); err != nil {
		return 0
	}
	if len(spans) == 0 {
		return 0
	}
	from := len(spans) - 1
	for i := len(spans) - 1; i > 0; i-- {
		if spans[i].start-spans[i-1].updated > playSittingAwaySeconds {
			break
		}
		from = i - 1
	}
	chain := spans[from:]
	wall := b.now().Unix() - chain[0].start
	if wall < 0 {
		wall = 0
	}
	ticks := 0
	for _, row := range chain {
		ticks += row.seconds
	}
	if int64(ticks) > wall {
		return ticks
	}
	return int(wall)
}

func (b *Bank) holdActiveLocked() bool {
	if b.ov.holdUntil.IsZero() {
		return false
	}
	return b.now().Before(b.ov.holdUntil)
}

func (b *Bank) stayUpActiveLocked() bool {
	if !b.holdActiveLocked() {
		return false
	}
	left, err := b.remainingLocked("fun")
	return err == nil && left > 0
}

func (b *Bank) setStayUpHoldLocked() error {
	until := b.nextWakeLocked()
	b.ov.holdUntil = until
	return b.metaSet(metaBedtimeHold, until.UTC().Format(time.RFC3339))
}

func (b *Bank) clearHoldLocked() error {
	b.ov.holdUntil = time.Time{}
	return b.metaSet(metaBedtimeHold, "")
}

func (b *Bank) grantStayUpLocked(actor *Token, group string, seconds int, reason, idemKey, source, bodyHash string) (*Grant, error) {
	g, err := b.postSetLocked(actor, group, seconds, reason, idemKey, source, bodyHash)
	if err != nil {
		return nil, err
	}
	if err := b.setStayUpHoldLocked(); err != nil {
		return nil, err
	}
	return g, nil
}

func (b *Bank) bedtimeStayUpLocked() bool {
	return b.effectiveBedtimeLockLocked() && b.bedtimeActiveLocked()
}

func (b *Bank) bedtimeActiveLocked() bool {
	start, end := b.effectiveBedtimeLocked()
	return config.ClockContains(b.cfg.Location, start, end, b.now())
}

func (b *Bank) nextWakeLocked() time.Time {
	_, end := b.effectiveBedtimeLocked()
	local := b.now().In(b.cfg.Location)
	wake := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, b.cfg.Location).Add(end)
	if !local.Before(wake) {
		wake = wake.Add(24 * time.Hour)
	}
	return wake
}

func (b *Bank) checkPinLocked(digits string) error {
	if b.ov.parentPin == "" {
		return fmt.Errorf("%w: parent pin is not set", ErrInvalid)
	}
	now := b.now()
	if now.Before(b.pinCool) {
		return fmt.Errorf("%w: too many pin attempts", ErrForbidden)
	}
	if pin.Verify(digits, b.ov.parentPin) {
		b.pinFails = 0
		b.pinCool = time.Time{}
		return nil
	}
	b.pinFails++
	if b.pinFails >= pinFailLimit {
		b.pinCool = now.Add(pinCoolFor)
		b.pinFails = 0
	}
	return fmt.Errorf("%w: invalid pin", ErrForbidden)
}

func requirePinActor(actor *Token) error {
	if actor == nil {
		return ErrUnauthorized
	}
	if actor.Kind != KindAsk && actor.Kind != KindRead {
		return ErrForbidden
	}
	return nil
}
