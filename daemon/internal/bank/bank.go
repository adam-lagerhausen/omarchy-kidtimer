package bank

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/look"

	_ "modernc.org/sqlite"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrNotFound     = errors.New("not found")
	ErrInvalid      = errors.New("invalid")
)

const (
	MaxAskSeconds      = 2 * 60 * 60
	MaxAbsGrantSeconds = 7 * 24 * 60 * 60
	defaultAppGrantCap = 600
	defaultAppDayCap   = 1800
	ledgerFileMode     = 0o600
)

type Kind string

const (
	KindParent Kind = "parent"
	KindRead   Kind = "read"
	KindAsk    Kind = "ask"
	KindApp    Kind = "app"
)

type Token struct {
	ID                 string
	Name               string
	Kind               Kind
	Groups             []string
	MaxSecondsPerGrant int
	MaxSecondsPerDay   int
}

type Grant struct {
	Group     string
	Seconds   int
	Source    string
	Reason    string
	Remaining int
	Replay    bool
}

type AskStatus string

const (
	AskPending  AskStatus = "pending"
	AskApproved AskStatus = "approved"
	AskDenied   AskStatus = "denied"
)

type Ask struct {
	ID      string    `json:"id"`
	Group   string    `json:"group"`
	Seconds int       `json:"seconds"`
	Reason  string    `json:"reason"`
	Status  AskStatus `json:"status"`
}

type SpendKind int

const (
	SpendBypass SpendKind = iota
	SpendTick
	SpendEmpty
)

type SpendResult struct {
	Kind      SpendKind
	Debited   string
	PathLeft  int
	GroupLeft int
}

type UntilKind int

const (
	UntilDefault UntilKind = iota
	UntilNext
	UntilFor
	UntilSticky
	UntilResume
)

type ModeApply struct {
	ID      string
	Kind    UntilKind
	Minutes int
}

type Status struct {
	KidName         string
	Groups          []GroupRemaining
	PathRemaining   map[string]int
	BedtimeActive   bool
	BedtimeIn       *int
	FocusedGroup    string
	FocusedApp      string
	PendingAskCount int
	Mode            string
	ParentLocked    bool
	RemoteLock      bool
	ParentPinSet    bool
	Overlay         bool
	BedtimeHold     bool
	BedtimeStart    string
	BedtimeEnd      string
	Hour12          bool
	Modes           map[string]map[string]int
	LookVersion     int
	Piles           []look.Pile
	OverrideUntil   time.Time
	Spent           map[string]int
	Today           []TodaySpan
}

type TodaySpan struct {
	Kind      string `json:"kind"`
	Start     int    `json:"start"`
	StartUnix int64  `json:"start_unix,omitempty"`
	Dur       int    `json:"dur"`
	Label     string `json:"label"`
}

type GroupRemaining struct {
	ID        string
	Policy    config.Policy
	Remaining int
}

type Paused struct {
	PID   int
	Group string
}

type FocusHit struct {
	Matched bool
	Bypass  bool
	Pile    string
	App     string
}

type Bank struct {
	mu         sync.Mutex
	db         *sql.DB
	cfg        *config.Config
	now        func() time.Time
	focused    string
	focusedApp string
	ov         overlay
	look       look.Document
	pinFails   int
	pinCool    time.Time
}

func Open(path string, cfg *config.Config, now func() time.Time) (*Bank, error) {
	if now == nil {
		now = time.Now
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := os.Chmod(path, ledgerFileMode); err != nil {
		_ = db.Close()
		return nil, err
	}
	b := &Bank{db: db, cfg: cfg, now: now}
	if err := b.loadOverlayLocked(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := b.ensureLookLocked(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := b.resetDayLocked(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return b, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS balances (
  group_id TEXT PRIMARY KEY,
  remaining INTEGER NOT NULL,
  day TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS grants (
  id INTEGER PRIMARY KEY,
  token_id TEXT NOT NULL,
  idempotency_key TEXT NOT NULL,
  body_hash TEXT NOT NULL,
  group_id TEXT NOT NULL,
  seconds INTEGER NOT NULL,
  source TEXT NOT NULL,
  reason TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE(token_id, idempotency_key)
);
CREATE TABLE IF NOT EXISTS asks (
  id TEXT PRIMARY KEY,
  group_id TEXT NOT NULL,
  seconds INTEGER NOT NULL,
  reason TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL,
  decided_at TEXT
);
CREATE TABLE IF NOT EXISTS tokens (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL,
  groups_json TEXT NOT NULL,
  max_seconds_per_grant INTEGER NOT NULL,
  max_seconds_per_day INTEGER NOT NULL,
  hash TEXT NOT NULL UNIQUE,
  created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS token_spend (
  token_id TEXT NOT NULL,
  day TEXT NOT NULL,
  seconds INTEGER NOT NULL,
  PRIMARY KEY (token_id, day)
);
CREATE TABLE IF NOT EXISTS paused (
  pid INTEGER PRIMARY KEY,
  group_id TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS daily_used (
  day TEXT NOT NULL,
  kind TEXT NOT NULL,
  seconds INTEGER NOT NULL,
  PRIMARY KEY (day, kind)
);
CREATE TABLE IF NOT EXISTS today_spans (
  id INTEGER PRIMARY KEY,
  day TEXT NOT NULL,
  start_min INTEGER NOT NULL,
  seconds INTEGER NOT NULL,
  label TEXT NOT NULL,
  updated_unix INTEGER NOT NULL,
  start_unix INTEGER NOT NULL DEFAULT 0
);
`)
	if err != nil {
		return err
	}
	_, _ = db.Exec(`ALTER TABLE today_spans ADD COLUMN start_unix INTEGER NOT NULL DEFAULT 0`)
	return nil
}

func (b *Bank) Close() error {
	return b.db.Close()
}

func (b *Bank) Config() *config.Config {
	return b.cfg
}

func (b *Bank) nowLocal() time.Time {
	return b.now().In(b.cfg.Location)
}

func (b *Bank) day() string {
	return b.nowLocal().Format("2006-01-02")
}

func (b *Bank) SeedParent(name string) (string, *Token, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return "", nil, err
	}
	return b.mintLocked(Token{
		Name: name,
		Kind: KindParent,
	})
}

type MintSpec struct {
	Name               string
	Kind               Kind
	Groups             []string
	MaxSecondsPerGrant int
	MaxSecondsPerDay   int
}

func (b *Bank) Mint(actor *Token, spec MintSpec) (string, *Token, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return "", nil, err
	}
	if actor == nil || actor.Kind != KindParent {
		return "", nil, ErrForbidden
	}
	tok := Token{
		Name:               spec.Name,
		Kind:               spec.Kind,
		Groups:             spec.Groups,
		MaxSecondsPerGrant: spec.MaxSecondsPerGrant,
		MaxSecondsPerDay:   spec.MaxSecondsPerDay,
	}
	return b.mintLocked(tok)
}

func (b *Bank) ReplaceToken(actor *Token, spec MintSpec) (string, *Token, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return "", nil, err
	}
	if actor == nil || actor.Kind != KindParent {
		return "", nil, ErrForbidden
	}
	tok := Token{
		Name:               spec.Name,
		Kind:               spec.Kind,
		Groups:             spec.Groups,
		MaxSecondsPerGrant: spec.MaxSecondsPerGrant,
		MaxSecondsPerDay:   spec.MaxSecondsPerDay,
	}
	secret, minted, err := b.mintLocked(tok)
	if err == nil {
		return secret, minted, nil
	}
	if !errors.Is(err, ErrConflict) {
		return "", nil, err
	}
	return b.replaceLocked(tok)
}

func (b *Bank) mintLocked(tok Token) (string, *Token, error) {
	if tok.Name == "" {
		return "", nil, fmt.Errorf("%w: name is required", ErrInvalid)
	}
	switch tok.Kind {
	case KindParent, KindRead, KindAsk, KindApp:
	default:
		return "", nil, fmt.Errorf("%w: kind must be parent, read, ask, or app", ErrInvalid)
	}
	groups, err := b.defaultGroups(tok.Kind, tok.Groups)
	if err != nil {
		return "", nil, err
	}
	tok.Groups = groups
	applyAppCaps(&tok)
	secret, err := randomSecret()
	if err != nil {
		return "", nil, err
	}
	tok.ID = newID()
	if _, err := b.db.Exec(
		`INSERT INTO tokens (id, name, kind, groups_json, max_seconds_per_grant, max_seconds_per_day, hash, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		tok.ID, tok.Name, string(tok.Kind), mustJSON(tok.Groups), tok.MaxSecondsPerGrant, tok.MaxSecondsPerDay, hashSecret(secret), b.nowLocal().Format(time.RFC3339),
	); err != nil {
		return "", nil, fmt.Errorf("%w: token name already exists", ErrConflict)
	}
	out := tok
	return secret, &out, nil
}

func (b *Bank) replaceLocked(tok Token) (string, *Token, error) {
	groups, err := b.defaultGroups(tok.Kind, tok.Groups)
	if err != nil {
		return "", nil, err
	}
	tok.Groups = groups
	applyAppCaps(&tok)
	secret, err := randomSecret()
	if err != nil {
		return "", nil, err
	}
	res, err := b.db.Exec(
		`UPDATE tokens SET kind = ?, groups_json = ?, max_seconds_per_grant = ?, max_seconds_per_day = ?, hash = ?
		 WHERE name = ?`,
		string(tok.Kind), mustJSON(tok.Groups), tok.MaxSecondsPerGrant, tok.MaxSecondsPerDay, hashSecret(secret), tok.Name,
	)
	if err != nil {
		return "", nil, err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return "", nil, fmt.Errorf("%w: token name already exists", ErrConflict)
	}
	err = b.db.QueryRow(`SELECT id FROM tokens WHERE name = ?`, tok.Name).Scan(&tok.ID)
	if err != nil {
		return "", nil, err
	}
	out := tok
	return secret, &out, nil
}

func applyAppCaps(tok *Token) {
	if tok == nil || tok.Kind != KindApp {
		return
	}
	if tok.MaxSecondsPerGrant <= 0 {
		tok.MaxSecondsPerGrant = defaultAppGrantCap
	}
	if tok.MaxSecondsPerDay <= 0 {
		tok.MaxSecondsPerDay = defaultAppDayCap
	}
}

func (b *Bank) defaultGroups(kind Kind, groups []string) ([]string, error) {
	switch kind {
	case KindRead, KindAsk:
		return nil, nil
	case KindParent:
		if len(groups) == 0 {
			return nil, nil
		}
		return b.checkGroups(groups)
	case KindApp:
		if len(groups) == 0 {
			if b.cfg.HasGroup("fun") {
				return []string{"fun"}, nil
			}
			return nil, fmt.Errorf("%w: app token must name groups when fun does not exist", ErrInvalid)
		}
		return b.checkGroups(groups)
	default:
		return nil, fmt.Errorf("%w: kind", ErrInvalid)
	}
}

func (b *Bank) checkGroups(groups []string) ([]string, error) {
	out := make([]string, 0, len(groups))
	seen := map[string]bool{}
	for _, id := range groups {
		if !b.cfg.HasGroup(id) && !b.look.HasPile(id) {
			return nil, fmt.Errorf("%w: unknown group %q", ErrInvalid, id)
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out, nil
}

func (b *Bank) Reclaim() (string, *Token, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return "", nil, err
	}
	v, ok, err := b.metaGet(metaPaired)
	if err != nil {
		return "", nil, err
	}
	if !ok || v != "true" {
		return "", nil, ErrConflict
	}
	return b.replaceLocked(Token{Name: "parent-pair", Kind: KindParent})
}

func (b *Bank) Pair() (string, *Token, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return "", nil, err
	}
	v, ok, err := b.metaGet(metaPaired)
	if err != nil {
		return "", nil, err
	}
	if ok && v == "true" {
		return "", nil, ErrConflict
	}
	secret, tok, err := b.mintLocked(Token{Name: "parent-pair", Kind: KindParent})
	if err != nil {
		return "", nil, err
	}
	if err := b.metaSet(metaPaired, "true"); err != nil {
		return "", nil, err
	}
	return secret, tok, nil
}

func (b *Bank) Paired() (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok, err := b.metaGet(metaPaired)
	if err != nil {
		return false, err
	}
	return ok && v == "true", nil
}

func (b *Bank) TokenCount() (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var n int
	err := b.db.QueryRow(`SELECT COUNT(*) FROM tokens`).Scan(&n)
	return n, err
}

func (b *Bank) LookupSecret(secret string) (*Token, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lookupHashLocked(hashSecret(secret))
}

func (b *Bank) lookupHashLocked(hash string) (*Token, error) {
	var tok Token
	var groupsJSON string
	var kind string
	err := b.db.QueryRow(
		`SELECT id, name, kind, groups_json, max_seconds_per_grant, max_seconds_per_day FROM tokens WHERE hash = ?`,
		hash,
	).Scan(&tok.ID, &tok.Name, &kind, &groupsJSON, &tok.MaxSecondsPerGrant, &tok.MaxSecondsPerDay)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUnauthorized
	}
	if err != nil {
		return nil, err
	}
	tok.Kind = Kind(kind)
	if err := json.Unmarshal([]byte(groupsJSON), &tok.Groups); err != nil {
		return nil, err
	}
	applyAppCaps(&tok)
	return &tok, nil
}

func (b *Bank) Grant(actor *Token, group string, seconds int, reason, idemKey string) (*Grant, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return nil, err
	}
	if actor == nil {
		return nil, ErrUnauthorized
	}
	applyAppCaps(actor)
	if actor.Kind != KindParent && actor.Kind != KindApp {
		return nil, ErrForbidden
	}
	if idemKey == "" {
		return nil, fmt.Errorf("%w: Idempotency-Key is required", ErrInvalid)
	}
	delta, err := parseGrantDelta(seconds)
	if err != nil {
		return nil, err
	}
	if !b.knownClockLocked(group) {
		return nil, fmt.Errorf("%w: unknown group %q", ErrInvalid, group)
	}
	if err := b.authorizeGrant(actor, group, delta); err != nil {
		return nil, err
	}
	bodyHash := grantBodyHash(group, delta, reason)
	var existing Grant
	var existingHash string
	err = b.db.QueryRow(
		`SELECT group_id, seconds, source, reason, body_hash FROM grants WHERE token_id = ? AND idempotency_key = ?`,
		actor.ID, idemKey,
	).Scan(&existing.Group, &existing.Seconds, &existing.Source, &existing.Reason, &existingHash)
	if err == nil {
		if existingHash != bodyHash {
			return nil, ErrConflict
		}
		remaining, err := b.remainingLocked(group)
		if err != nil {
			return nil, err
		}
		existing.Remaining = remaining
		existing.Replay = true
		return &existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	g, err := b.postLocked(actor, group, delta, reason, idemKey, grantSource(actor, ""), bodyHash)
	if err != nil {
		return nil, err
	}
	if actor.Kind == KindApp && delta > 0 {
		if err := b.addTokenSpendLocked(actor.ID, delta); err != nil {
			return nil, err
		}
	}
	return g, nil
}

func parseGrantDelta(seconds int) (int, error) {
	if seconds == 0 {
		return 0, fmt.Errorf("%w: seconds must be nonzero", ErrInvalid)
	}
	if seconds > MaxAbsGrantSeconds || seconds < -MaxAbsGrantSeconds {
		return 0, fmt.Errorf("%w: seconds exceeds %d", ErrInvalid, MaxAbsGrantSeconds)
	}
	return seconds, nil
}

func (b *Bank) authorizeGrant(actor *Token, group string, seconds int) error {
	if seconds < 0 && actor.Kind != KindParent {
		return ErrForbidden
	}
	if err := actorCanCredit(actor, group); err != nil {
		return err
	}
	if actor.Kind != KindApp || seconds <= 0 {
		return nil
	}
	if actor.MaxSecondsPerGrant > 0 && seconds > actor.MaxSecondsPerGrant {
		return fmt.Errorf("%w: exceeds max_seconds_per_grant", ErrForbidden)
	}
	used, err := b.tokenSpentLocked(actor.ID)
	if err != nil {
		return err
	}
	if actor.MaxSecondsPerDay > 0 && used+seconds > actor.MaxSecondsPerDay {
		return fmt.Errorf("%w: exceeds max_seconds_per_day", ErrForbidden)
	}
	return nil
}

func applyDeltaLocked(tx *sql.Tx, group string, seconds int) error {
	_, err := tx.Exec(`UPDATE balances SET remaining = MAX(0, remaining + ?) WHERE group_id = ?`, seconds, group)
	return err
}

func (b *Bank) postLocked(actor *Token, group string, seconds int, reason, idemKey, source, bodyHash string) (*Grant, error) {
	tx, err := b.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO grants (token_id, idempotency_key, body_hash, group_id, seconds, source, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		actor.ID, idemKey, bodyHash, group, seconds, source, reason, b.nowLocal().Format(time.RFC3339),
	); err != nil {
		return nil, err
	}
	if err := applyDeltaLocked(tx, group, seconds); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	remaining, err := b.remainingLocked(group)
	if err != nil {
		return nil, err
	}
	return &Grant{Group: group, Seconds: seconds, Source: source, Reason: reason, Remaining: remaining}, nil
}

func (b *Bank) postSetLocked(actor *Token, group string, seconds int, reason, idemKey, source, bodyHash string) (*Grant, error) {
	tx, err := b.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`INSERT INTO grants (token_id, idempotency_key, body_hash, group_id, seconds, source, reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		actor.ID, idemKey, bodyHash, group, seconds, source, reason, b.nowLocal().Format(time.RFC3339),
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`INSERT INTO balances (group_id, remaining, day) VALUES (?, ?, ?)
		 ON CONFLICT(group_id) DO UPDATE SET remaining = excluded.remaining, day = excluded.day`,
		group, seconds, b.day(),
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Grant{Group: group, Seconds: seconds, Source: source, Reason: reason, Remaining: seconds}, nil
}

func actorCanCredit(actor *Token, group string) error {
	if actor.Kind == KindParent {
		return nil
	}
	if len(actor.Groups) == 0 {
		return ErrForbidden
	}
	for _, id := range actor.Groups {
		if id == group {
			return nil
		}
	}
	return fmt.Errorf("%w: token cannot credit %q", ErrForbidden, group)
}

func grantSource(actor *Token, askID string) string {
	if askID != "" {
		return "ask:" + askID
	}
	switch actor.Kind {
	case KindParent:
		return "parent"
	case KindApp:
		return "app:" + actor.Name
	default:
		return string(actor.Kind)
	}
}

func (b *Bank) CreateAsk(actor *Token, group string, seconds int, reason string) (*Ask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return nil, err
	}
	if actor == nil {
		return nil, ErrUnauthorized
	}
	if actor.Kind != KindAsk {
		return nil, ErrForbidden
	}
	if seconds <= 0 {
		return nil, fmt.Errorf("%w: seconds must be positive", ErrInvalid)
	}
	if seconds > MaxAskSeconds {
		return nil, fmt.Errorf("%w: seconds exceeds %d", ErrInvalid, MaxAskSeconds)
	}
	if !b.knownClockLocked(group) {
		return nil, fmt.Errorf("%w: unknown group %q", ErrInvalid, group)
	}
	ask := Ask{ID: newID(), Group: group, Seconds: seconds, Reason: reason, Status: AskPending}
	if _, err := b.db.Exec(
		`INSERT INTO asks (id, group_id, seconds, reason, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		ask.ID, ask.Group, ask.Seconds, ask.Reason, string(ask.Status), b.nowLocal().Format(time.RFC3339),
	); err != nil {
		return nil, err
	}
	return &ask, nil
}

func (b *Bank) PendingAsks(actor *Token) ([]Ask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if actor == nil {
		return nil, ErrUnauthorized
	}
	if actor.Kind != KindParent {
		return nil, ErrForbidden
	}
	rows, err := b.db.Query(`SELECT id, group_id, seconds, reason, status FROM asks WHERE status = ? ORDER BY created_at`, string(AskPending))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ask
	for rows.Next() {
		var a Ask
		var status string
		if err := rows.Scan(&a.ID, &a.Group, &a.Seconds, &a.Reason, &status); err != nil {
			return nil, err
		}
		a.Status = AskStatus(status)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (b *Bank) Decide(actor *Token, askID, decision string) (*Grant, *Ask, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return nil, nil, err
	}
	if actor == nil || actor.Kind != KindParent {
		return nil, nil, ErrForbidden
	}
	return b.decideLocked(actor, askID, decision)
}

func (b *Bank) decideLocked(actor *Token, askID, decision string) (*Grant, *Ask, error) {
	var ask Ask
	var status string
	err := b.db.QueryRow(`SELECT id, group_id, seconds, reason, status FROM asks WHERE id = ?`, askID).
		Scan(&ask.ID, &ask.Group, &ask.Seconds, &ask.Reason, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	ask.Status = AskStatus(status)
	if ask.Status != AskPending {
		return nil, nil, ErrConflict
	}
	now := b.nowLocal().Format(time.RFC3339)
	switch decision {
	case "deny":
		if _, err := b.db.Exec(`UPDATE asks SET status = ?, decided_at = ? WHERE id = ?`, string(AskDenied), now, askID); err != nil {
			return nil, nil, err
		}
		ask.Status = AskDenied
		return nil, &ask, nil
	case "approve":
		source := "ask:" + ask.ID
		idemKey := "ask:" + ask.ID
		bodyHash := grantBodyHash(ask.Group, ask.Seconds, ask.Reason)
		var g *Grant
		var err error
		if b.bedtimeStayUpLocked() {
			g, err = b.grantStayUpLocked(actor, ask.Group, ask.Seconds, ask.Reason, idemKey, source, bodyHash)
		} else {
			g, err = b.postLocked(actor, ask.Group, ask.Seconds, ask.Reason, idemKey, source, bodyHash)
		}
		if err != nil {
			return nil, nil, err
		}
		if _, err := b.db.Exec(`UPDATE asks SET status = ?, decided_at = ? WHERE id = ?`, string(AskApproved), now, askID); err != nil {
			return nil, nil, err
		}
		ask.Status = AskApproved
		return g, &ask, nil
	default:
		return nil, nil, fmt.Errorf("%w: decision must be approve or deny", ErrInvalid)
	}
}

func (b *Bank) Remaining(group string) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return 0, err
	}
	return b.remainingLocked(group)
}

func (b *Bank) remainingLocked(group string) (int, error) {
	var n int
	err := b.db.QueryRow(`SELECT remaining FROM balances WHERE group_id = ?`, group).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("%w: unknown group %q", ErrInvalid, group)
	}
	return n, err
}

func (b *Bank) SpendPathRemaining(group string) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return 0, err
	}
	return b.spendPathLocked(group)
}

func (b *Bank) spendPathLocked(group string) (int, error) {
	if g, ok := b.cfg.Group(group); ok && g.Policy == config.Bypass {
		return 1, nil
	}
	if !b.knownClockLocked(group) {
		return 0, fmt.Errorf("%w: unknown group %q", ErrInvalid, group)
	}
	return b.remainingLocked(group)
}

func (b *Bank) Spend(group string) (*SpendResult, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return nil, err
	}
	if g, ok := b.cfg.Group(group); ok && g.Policy == config.Bypass {
		return &SpendResult{Kind: SpendBypass, PathLeft: 1}, nil
	}
	if !b.knownClockLocked(group) {
		return nil, fmt.Errorf("%w: unknown group %q", ErrInvalid, group)
	}
	own, err := b.remainingLocked(group)
	if err != nil {
		return nil, err
	}
	if own > 0 {
		if _, err := b.db.Exec(`UPDATE balances SET remaining = remaining - 1 WHERE group_id = ?`, group); err != nil {
			return nil, err
		}
		left := own - 1
		if left == 0 && group == "fun" {
			if err := b.clearHoldLocked(); err != nil {
				return nil, err
			}
			if err := b.applyDeferredRefillLocked(); err != nil {
				return nil, err
			}
		}
		return &SpendResult{Kind: SpendTick, Debited: group, PathLeft: left, GroupLeft: left}, nil
	}
	return &SpendResult{Kind: SpendEmpty, PathLeft: 0, GroupLeft: 0}, nil
}

func (b *Bank) BedtimeActive() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	start, end := b.effectiveBedtimeLocked()
	return config.ClockContains(b.cfg.Location, start, end, b.now())
}

func (b *Bank) prepareLocked() error {
	if err := b.resetDayLocked(); err != nil {
		return err
	}
	return b.syncScheduleLocked()
}

func (b *Bank) SetFocused(group string) {
	b.SetFocus(group, "")
}

func (b *Bank) SetFocus(pile, app string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.focused = pile
	b.focusedApp = app
}

func (b *Bank) FocusHit(class, title string) FocusHit {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.focusHitLocked(class, title)
}

func (b *Bank) focusHitLocked(class, title string) FocusHit {
	if !b.knownClockLocked("fun") {
		return FocusHit{}
	}
	hit := FocusHit{Matched: true, Pile: "fun"}
	if t, ok := look.Resolve(b.look, class, title); ok {
		hit.App = strings.ToLower(t.Name)
	}
	return hit
}

func (b *Bank) Status(actor *Token) (*Status, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return nil, err
	}
	if actor == nil {
		return nil, ErrUnauthorized
	}
	if actor.Kind != KindParent && actor.Kind != KindRead {
		return nil, ErrForbidden
	}
	start, end := b.effectiveBedtimeLocked()
	st := &Status{
		KidName:       b.cfg.KidName,
		PathRemaining: map[string]int{},
		BedtimeActive: config.ClockContains(b.cfg.Location, start, end, b.now()),
		FocusedGroup:  b.focused,
		FocusedApp:    b.focusedApp,
		Mode:          b.ov.activeMode,
		LookVersion:   b.look.Version,
		ParentLocked:  b.ov.parentLock,
		RemoteLock:    b.cfg.RemoteLock,
		ParentPinSet:  b.ov.parentPin != "",
		Overlay:       b.overlayActiveLocked(),
		BedtimeHold:   b.stayUpActiveLocked(),
		BedtimeStart:  config.FormatClock(start),
		BedtimeEnd:    config.FormatClock(end),
		Hour12:        b.hour12Locked(),
		Modes:         b.effectiveModesLocked(),
		Piles:         append([]look.Pile{}, b.look.Piles...),
		OverrideUntil: b.ov.overrideUntil,
	}
	if b.effectiveBedtimeLockLocked() {
		n := 0
		if !st.BedtimeActive {
			n = config.SecondsUntil(b.cfg.Location, start, b.now())
		}
		st.BedtimeIn = &n
	}
	seen := map[string]bool{}
	for i := range b.cfg.Groups {
		g := &b.cfg.Groups[i]
		n, err := b.remainingLocked(g.ID)
		if err != nil {
			return nil, err
		}
		st.Groups = append(st.Groups, GroupRemaining{ID: g.ID, Policy: g.Policy, Remaining: n})
		seen[g.ID] = true
		if g.Policy != config.Metered {
			continue
		}
		st.PathRemaining[g.ID] = n
	}
	for _, p := range b.look.Piles {
		if seen[p.ID] {
			continue
		}
		n, err := b.remainingLocked(p.ID)
		if err != nil {
			return nil, err
		}
		st.Groups = append(st.Groups, GroupRemaining{ID: p.ID, Policy: config.Metered, Remaining: n})
		st.PathRemaining[p.ID] = n
	}
	if err := b.db.QueryRow(`SELECT COUNT(*) FROM asks WHERE status = ?`, string(AskPending)).Scan(&st.PendingAskCount); err != nil {
		return nil, err
	}
	st.Spent = map[string]int{
		"fun": b.spentLocked("fun"),
	}
	st.Today = b.todayLocked()
	return st, nil
}

func (b *Bank) AddSpent(kind string, n int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.addSpentLocked(kind, n)
}

func (b *Bank) addSpentLocked(kind string, n int) error {
	if kind != "fun" {
		return fmt.Errorf("%w: unknown spent %q", ErrInvalid, kind)
	}
	day := b.day()
	_, err := b.db.Exec(`INSERT INTO daily_used (day, kind, seconds) VALUES (?, ?, ?)
		ON CONFLICT(day, kind) DO UPDATE SET seconds = daily_used.seconds + excluded.seconds`, day, kind, n)
	return err
}

func (b *Bank) spentLocked(kind string) int {
	var n int
	err := b.db.QueryRow(`SELECT seconds FROM daily_used WHERE day = ? AND kind = ?`, b.day(), kind).Scan(&n)
	if err != nil {
		return 0
	}
	return n
}

const todayGapSeconds = 3

func (b *Bank) NoteToday(label string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return err
	}
	return b.noteTodayLocked(label)
}

func (b *Bank) noteTodayLocked(label string) error {
	label = strings.ToLower(strings.TrimSpace(label))
	if label == "" {
		label = "on"
	}
	now := b.nowLocal()
	day := b.day()
	startMin := now.Hour()*60 + now.Minute()
	unix := now.Unix()
	var id int
	var lastLabel string
	var lastUnix int64
	err := b.db.QueryRow(`SELECT id, label, updated_unix FROM today_spans WHERE day = ? ORDER BY id DESC LIMIT 1`, day).Scan(&id, &lastLabel, &lastUnix)
	if err == nil && lastLabel == label && unix-lastUnix <= todayGapSeconds {
		_, err = b.db.Exec(`UPDATE today_spans SET seconds = seconds + 1, updated_unix = ? WHERE id = ?`, unix, id)
		return err
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = b.db.Exec(`INSERT INTO today_spans (day, start_min, seconds, label, updated_unix, start_unix) VALUES (?, ?, 1, ?, ?, ?)`, day, startMin, label, unix, unix)
	return err
}

func (b *Bank) spanStartUnix(day string, startMin int, stored int64) int64 {
	if stored > 0 {
		return stored
	}
	t, err := time.ParseInLocation("2006-01-02", day, b.cfg.Location)
	if err != nil {
		return 0
	}
	return t.Add(time.Duration(startMin) * time.Minute).Unix()
}

func (b *Bank) todayLocked() []TodaySpan {
	out := []TodaySpan{}
	day := b.day()
	rows, err := b.db.Query(`SELECT start_min, seconds, label, start_unix FROM today_spans WHERE day = ? ORDER BY id`, day)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var start, seconds int
		var label string
		var startUnix int64
		if err := rows.Scan(&start, &seconds, &label, &startUnix); err != nil {
			return out
		}
		dur := (seconds + 59) / 60
		if dur < 1 {
			dur = 1
		}
		out = append(out, TodaySpan{
			Kind:      "on",
			Start:     start,
			StartUnix: b.spanStartUnix(day, start, startUnix),
			Dur:       dur,
			Label:     label,
		})
	}
	return out
}

func (b *Bank) RecordPaused(pid int, group string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err := b.db.Exec(`INSERT INTO paused (pid, group_id) VALUES (?, ?) ON CONFLICT(pid) DO UPDATE SET group_id = excluded.group_id`, pid, group)
	return err
}

func (b *Bank) ClearPaused(pid int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err := b.db.Exec(`DELETE FROM paused WHERE pid = ?`, pid)
	return err
}

func (b *Bank) IsPaused(pid int) (string, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	var group string
	err := b.db.QueryRow(`SELECT group_id FROM paused WHERE pid = ?`, pid).Scan(&group)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return group, true, nil
}

func (b *Bank) Paused() ([]Paused, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	rows, err := b.db.Query(`SELECT pid, group_id FROM paused`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Paused
	for rows.Next() {
		var p Paused
		if err := rows.Scan(&p.PID, &p.Group); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (b *Bank) PausedToResume(credited string) ([]Paused, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.prepareLocked(); err != nil {
		return nil, err
	}
	rows, err := b.db.Query(`SELECT pid, group_id FROM paused`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Paused
	for rows.Next() {
		var p Paused
		if err := rows.Scan(&p.PID, &p.Group); err != nil {
			return nil, err
		}
		if !resumeEligible(p.Group, credited) {
			continue
		}
		left, err := b.spendPathLocked(p.Group)
		if err != nil {
			return nil, err
		}
		if left > 0 {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}

func resumeEligible(pausedGroup, credited string) bool {
	return pausedGroup == credited
}

func (b *Bank) resetDayLocked() error {
	day := b.day()
	var stored sql.NullString
	if err := b.db.QueryRow(`SELECT value FROM meta WHERE key = 'day'`).Scan(&stored); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if stored.Valid && stored.String == day {
		if err := b.ensureBalancesLocked(day); err != nil {
			return err
		}
		if b.stayUpActiveLocked() {
			return nil
		}
		return b.applyDeferredRefillLocked()
	}
	if _, err := b.db.Exec(`DELETE FROM daily_used WHERE day != ?`, day); err != nil {
		return err
	}
	if _, err := b.db.Exec(`DELETE FROM today_spans WHERE day != ?`, day); err != nil {
		return err
	}
	if err := b.setDayLocked(day); err != nil {
		return err
	}
	if b.stayUpActiveLocked() {
		return b.metaSet(metaRefillDeferred, "true")
	}
	if err := b.refillClocksLocked(day); err != nil {
		return err
	}
	return b.metaSet(metaRefillDeferred, "false")
}

func (b *Bank) setDayLocked(day string) error {
	_, err := b.db.Exec(`INSERT INTO meta (key, value) VALUES ('day', ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, day)
	return err
}

func (b *Bank) refillClocksLocked(day string) error {
	for _, id := range b.clockIDsLocked() {
		if err := b.writeRemainingLocked(id, b.allotmentLocked(id), day); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bank) applyDeferredRefillLocked() error {
	v, ok, err := b.metaGet(metaRefillDeferred)
	if err != nil {
		return err
	}
	if !ok || v != "true" {
		return nil
	}
	if err := b.refillClocksLocked(b.day()); err != nil {
		return err
	}
	return b.metaSet(metaRefillDeferred, "false")
}

func (b *Bank) ensureBalancesLocked(day string) error {
	for _, id := range b.clockIDsLocked() {
		var exists int
		err := b.db.QueryRow(`SELECT 1 FROM balances WHERE group_id = ?`, id).Scan(&exists)
		if err == nil {
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err := b.writeRemainingLocked(id, b.allotmentLocked(id), day); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bank) clockIDsLocked() []string {
	seen := map[string]bool{}
	var ids []string
	for i := range b.cfg.Groups {
		id := b.cfg.Groups[i].ID
		ids = append(ids, id)
		seen[id] = true
	}
	for _, p := range b.look.Piles {
		if seen[p.ID] {
			continue
		}
		ids = append(ids, p.ID)
	}
	return ids
}

func (b *Bank) knownClockLocked(id string) bool {
	if b.cfg.HasGroup(id) {
		return true
	}
	return b.look.HasPile(id)
}

func (b *Bank) tokenSpentLocked(tokenID string) (int, error) {
	var n sql.NullInt64
	err := b.db.QueryRow(`SELECT seconds FROM token_spend WHERE token_id = ? AND day = ?`, tokenID, b.day()).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return int(n.Int64), nil
}

func (b *Bank) addTokenSpendLocked(tokenID string, seconds int) error {
	_, err := b.db.Exec(
		`INSERT INTO token_spend (token_id, day, seconds) VALUES (?, ?, ?)
		 ON CONFLICT(token_id, day) DO UPDATE SET seconds = seconds + excluded.seconds`,
		tokenID, b.day(), seconds,
	)
	return err
}

func grantBodyHash(group string, seconds int, reason string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\n%d\n%s", group, seconds, reason)))
	return hex.EncodeToString(sum[:])
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func randomSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}
