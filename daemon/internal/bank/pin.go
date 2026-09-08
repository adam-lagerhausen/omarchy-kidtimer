package bank

import (
	"fmt"
	"time"

	"kidtimer/daemon/internal/config"
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
	if err := b.clearParentLockLocked(); err != nil {
		return nil, err
	}
	return g, nil
}

func (b *Bank) OverlayActive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.overlayActiveLocked()
}

func (b *Bank) overlayActiveLocked() bool {
	if b.ov.parentPin == "" {
		return false
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
