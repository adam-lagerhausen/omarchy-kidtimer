package bank

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"kidtimer/daemon/internal/config"
	"kidtimer/daemon/internal/look"
)

func TestGrantAndCap(t *testing.T) {
	b, parent := openTest(t, afternoon)
	if remaining(t, b, "fun") != 3600 {
		t.Fatal("fun starts at 3600")
	}
	g, err := b.Grant(parent, "fun", 900, "good afternoon", "k1")
	if err != nil {
		t.Fatal(err)
	}
	if g.Source != "parent" || g.Remaining != 4500 {
		t.Fatalf("grant: %+v", g)
	}
	g, err = b.Grant(parent, "fun", 900, "+15", "k2")
	if err != nil {
		t.Fatal(err)
	}
	if g.Remaining != 5400 {
		t.Fatalf("fun remaining: %d", g.Remaining)
	}
}

func TestGrantSignedDelta(t *testing.T) {
	b, parent := openTest(t, afternoon)
	if remaining(t, b, "fun") != 3600 {
		t.Fatalf("fun seed: %d", remaining(t, b, "fun"))
	}
	g, err := b.Grant(parent, "fun", -3600, "-10", "debit-10")
	if err != nil {
		t.Fatal(err)
	}
	if g.Seconds != -3600 || g.Remaining != 0 || g.Source != "parent" || g.Reason != "-10" {
		t.Fatalf("debit: %+v", g)
	}
	sec, source, reason := grantRow(t, b, "debit-10")
	if sec != -3600 || source != "parent" || reason != "-10" {
		t.Fatalf("row: seconds=%d source=%s reason=%s", sec, source, reason)
	}

	atZero, err := b.Grant(parent, "fun", -600, "-10", "debit-at-0")
	if err != nil {
		t.Fatal(err)
	}
	if atZero.Remaining != 0 || remaining(t, b, "fun") != 0 {
		t.Fatalf("debit at 0: %+v remaining=%d", atZero, remaining(t, b, "fun"))
	}
	if grantCount(t, b, "debit-at-0") != 1 {
		t.Fatal("debit at 0 must insert a row")
	}

	replay, err := b.Grant(parent, "fun", -600, "-10", "debit-at-0")
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || remaining(t, b, "fun") != 0 {
		t.Fatalf("replay: %+v remaining=%d", replay, remaining(t, b, "fun"))
	}
	if grantCount(t, b, "debit-at-0") != 1 {
		t.Fatal("replay must not insert another row")
	}

	if _, err := b.Grant(parent, "fun", 600, "+10", "sign-key"); err != nil {
		t.Fatal(err)
	}
	_, err = b.Grant(parent, "fun", -600, "-10", "sign-key")
	if err != ErrConflict {
		t.Fatalf("600 vs -600: %v", err)
	}

	_, app, err := b.Mint(parent, MintSpec{Name: "khan-webhook", Kind: KindApp})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.Grant(app, "fun", -600, "-10", "app-debit")
	if err != ErrForbidden {
		t.Fatalf("app debit: %v", err)
	}

	_, askTok, err := b.Mint(parent, MintSpec{Name: "kid-bar", Kind: KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	_, err = b.CreateAsk(askTok, "fun", -600, "-10")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("ask debit: %v", err)
	}

	credit, err := b.Grant(parent, "fun", 600, "+10", "parent-plus10")
	if err != nil {
		t.Fatal(err)
	}
	if credit.Seconds != 600 || credit.Remaining != 1200 {
		t.Fatalf("parent credit: %+v", credit)
	}
}

func TestMidnightResetDropsYesterdayGrants(t *testing.T) {
	clock := &clock{t: afternoon()}
	b, parent := openTestClock(t, clock.now)
	if _, err := b.Grant(parent, "fun", 900, "yesterday", "k1"); err != nil {
		t.Fatal(err)
	}
	clock.t = afternoon().Add(24 * time.Hour)
	if remaining(t, b, "fun") != 3600 {
		t.Fatal("fun extras must not stack past midnight")
	}
}

func TestFunSpendsOwnClock(t *testing.T) {
	b, _ := openTest(t, afternoon)
	left := remaining(t, b, "fun")
	spendN(t, b, "fun", left)
	res, err := b.Spend("fun")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != SpendEmpty {
		t.Fatalf("fun empty: %+v", res)
	}
}

func TestNoMatchIsConfigLevel(t *testing.T) {
	b, _ := openTest(t, afternoon)
	if _, ok := b.Config().Match("Alacritty", ""); ok {
		t.Fatal("unknown should not match")
	}
}

func TestAnyWindowHitsTheClock(t *testing.T) {
	b, _ := openTest(t, afternoon)
	hit := b.FocusHit("google-chrome", "")
	if !hit.Matched || hit.Pile != "fun" || hit.Bypass {
		t.Fatalf("chrome: %+v", hit)
	}
	lab := b.FocusHit("kidtimer-lab", "")
	if !lab.Matched || lab.Pile != "fun" {
		t.Fatalf("lab fun: %+v", lab)
	}
}

func TestIdempotencyReplayAndConflict(t *testing.T) {
	b, parent := openTest(t, afternoon)
	first, err := b.Grant(parent, "fun", 60, "same", "idem-1")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := b.Grant(parent, "fun", 60, "same", "idem-1")
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay || remaining(t, b, "fun") != first.Remaining {
		t.Fatalf("replay doubled: %+v remaining=%d", replay, remaining(t, b, "fun"))
	}
	_, err = b.Grant(parent, "fun", 120, "different", "idem-1")
	if err != ErrConflict {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestTokenScope(t *testing.T) {
	b, parent := openTest(t, afternoon)
	secret, app, err := b.Mint(parent, MintSpec{
		Name:               "khan-webhook",
		Kind:               KindApp,
		MaxSecondsPerGrant: 600,
		MaxSecondsPerDay:   1800,
	})
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" {
		t.Fatal("secret shown once")
	}
	if len(app.Groups) != 1 || app.Groups[0] != "fun" {
		t.Fatalf("app default groups: %v", app.Groups)
	}
	g, err := b.Grant(app, "fun", 60, "lesson", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if g.Source != "app:khan-webhook" {
		t.Fatalf("source: %q", g.Source)
	}
	replay, err := b.Grant(app, "fun", 60, "lesson", "a1")
	if err != nil {
		t.Fatal(err)
	}
	if !replay.Replay {
		t.Fatal("expected replay")
	}
	_, err = b.Grant(app, "missing", 60, "nope", "a2")
	if err == nil {
		t.Fatal("app must not credit an unknown group")
	}
	_, err = b.Grant(app, "fun", 601, "too big", "a3")
	if err == nil {
		t.Fatal("max_seconds_per_grant")
	}
	if _, err := b.Grant(app, "fun", 600, "chunk", "a4"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Grant(app, "fun", 600, "chunk2", "a5"); err != nil {
		t.Fatal(err)
	}
	_, err = b.Grant(app, "fun", 600, "chunk3", "a6")
	if err == nil {
		t.Fatal("max_seconds_per_day")
	}
}

func TestAppGrantDoesNotSkipBedtime(t *testing.T) {
	b, parent := openTest(t, bedtime)
	_, app, err := b.Mint(parent, MintSpec{Name: "math", Kind: KindApp})
	if err != nil {
		t.Fatal(err)
	}
	if !b.BedtimeActive() {
		t.Fatal("fixture should be inside bedtime")
	}
	if _, err := b.Grant(app, "fun", 600, "cannot skip", "bed"); err != nil {
		t.Fatal(err)
	}
	if !b.BedtimeActive() {
		t.Fatal("app grant must not lift bedtime")
	}
	if _, err := b.Grant(parent, "fun", 900, "parent extra", "bed2"); err != nil {
		t.Fatal(err)
	}
	if !b.BedtimeActive() {
		t.Fatal("parent grant must not skip bedtime")
	}
}

func TestAskDoesNotCreditUntilDecide(t *testing.T) {
	b, parent := openTest(t, afternoon)
	_, askTok, err := b.Mint(parent, MintSpec{Name: "kid-bar", Kind: KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := b.CreateAsk(askTok, "fun", 900, "one more video")
	if err != nil {
		t.Fatal(err)
	}
	if remaining(t, b, "fun") != 3600 {
		t.Fatal("ask credited early")
	}
	st, err := b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if st.PendingAskCount != 1 {
		t.Fatalf("pending: %d", st.PendingAskCount)
	}
	grant, decided, err := b.Decide(parent, ask.ID, "approve")
	if err != nil {
		t.Fatal(err)
	}
	if grant.Source != "ask:"+ask.ID || decided.Status != AskApproved {
		t.Fatalf("approve: %+v %+v", grant, decided)
	}
	if remaining(t, b, "fun") != 4500 {
		t.Fatalf("fun after approve: %d", remaining(t, b, "fun"))
	}
	_, _, err = b.Decide(parent, ask.ID, "approve")
	if err != ErrConflict {
		t.Fatalf("second decide: %v", err)
	}
	if remaining(t, b, "fun") != 4500 {
		t.Fatal("second decide granted twice")
	}
}

func TestDenyAskDoesNotGrant(t *testing.T) {
	b, parent := openTest(t, afternoon)
	_, askTok, err := b.Mint(parent, MintSpec{Name: "kid-bar", Kind: KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	ask, err := b.CreateAsk(askTok, "fun", 900, "no")
	if err != nil {
		t.Fatal(err)
	}
	grant, decided, err := b.Decide(parent, ask.ID, "deny")
	if err != nil {
		t.Fatal(err)
	}
	if grant != nil || decided.Status != AskDenied {
		t.Fatalf("deny: %+v %+v", grant, decided)
	}
	if remaining(t, b, "fun") != 3600 {
		t.Fatal("deny credited")
	}
}

func TestPausedResumeMatchedPileOnly(t *testing.T) {
	b, parent := openTest(t, afternoon)
	if err := b.RecordPaused(4242, "fun"); err != nil {
		t.Fatal(err)
	}
	spendN(t, b, "fun", remaining(t, b, "fun"))
	got, err := b.PausedToResume("other")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatal("other must not resume fun")
	}
	if _, err := b.Grant(parent, "fun", 30, "own clock", "fun-resume"); err != nil {
		t.Fatal(err)
	}
	got, err = b.PausedToResume("fun")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].PID != 4242 {
		t.Fatalf("resume: %+v", got)
	}
}

func TestReadTokenCannotGrant(t *testing.T) {
	b, parent := openTest(t, afternoon)
	_, read, err := b.Mint(parent, MintSpec{Name: "bar", Kind: KindRead})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Grant(read, "fun", 60, "no", "x"); err == nil {
		t.Fatal("read token granted")
	}
	if _, err := b.Status(read); err != nil {
		t.Fatal(err)
	}
}

func TestStatusPathRemainingAndBedtimeIn(t *testing.T) {
	cfg, err := config.ParseFile(packagingPath(t, "config.kid.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.BedtimeLock {
		t.Fatal("kid fixture must lock bedtime")
	}
	b, err := Open(filepath.Join(t.TempDir(), "ledger.sqlite"), cfg, afternoon)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	_, parent, err := b.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Grant(parent, "fun", 120, "bonus", "k1"); err != nil {
		t.Fatal(err)
	}
	st, err := b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st.PathRemaining["school"]; ok {
		t.Fatal("bypass must be omitted from path_remaining")
	}
	if st.PathRemaining["fun"] != 3720 {
		t.Fatalf("fun path: %d", st.PathRemaining["fun"])
	}
	if st.LookVersion < 1 {
		t.Fatal("look_version")
	}
	if st.BedtimeIn == nil || *st.BedtimeIn != 6*3600 {
		t.Fatalf("bedtime_in: %v", st.BedtimeIn)
	}
	if st.Today == nil || len(st.Today) != 0 {
		t.Fatalf("today: %+v", st.Today)
	}

	night, err := Open(filepath.Join(t.TempDir(), "night.sqlite"), cfg, bedtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = night.Close() })
	_, nightParent, err := night.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	st, err = night.Status(nightParent)
	if err != nil {
		t.Fatal(err)
	}
	if !st.BedtimeActive || st.BedtimeIn == nil || *st.BedtimeIn != 0 {
		t.Fatalf("active bedtime_in: active=%v in=%v", st.BedtimeActive, st.BedtimeIn)
	}
}

func TestTodaySpansMergeAcrossTicksAndSplitOnGap(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	c := &clock{t: time.Date(2026, 8, 26, 15, 0, 0, 0, loc)}
	b, parent := openTestClock(t, c.now)
	if err := b.NoteToday("chrome"); err != nil {
		t.Fatal(err)
	}
	c.t = c.t.Add(time.Second)
	if err := b.NoteToday("chrome"); err != nil {
		t.Fatal(err)
	}
	st, err := b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Today) != 1 || st.Today[0].Label != "chrome" || st.Today[0].Start != 15*60 {
		t.Fatalf("merged: %+v", st.Today)
	}
	c.t = c.t.Add(10 * time.Second)
	if err := b.NoteToday("chrome"); err != nil {
		t.Fatal(err)
	}
	if err := b.NoteToday("minecraft"); err != nil {
		t.Fatal(err)
	}
	st, err = b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Today) != 3 {
		t.Fatalf("gap and app change: %+v", st.Today)
	}
	if st.Today[1].Label != "chrome" || st.Today[2].Label != "minecraft" {
		t.Fatalf("labels: %+v", st.Today)
	}
}

func TestMidnightUsesSaturdayHours(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	clock := &clock{t: time.Date(2026, 8, 29, 10, 0, 0, 0, loc)}
	b, _ := openTestClock(t, clock.now)
	if remaining(t, b, "fun") != 7200 {
		t.Fatalf("saturday hours: %d", remaining(t, b, "fun"))
	}
}

func TestPutLookLeavesRemaining(t *testing.T) {
	b, parent := openTest(t, afternoon)
	if _, err := b.Grant(parent, "fun", 90, "keep", "keep"); err != nil {
		t.Fatal(err)
	}
	doc, err := b.Look(parent)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Piles) == 0 || doc.Piles[0].ID != "fun" || doc.Piles[0].Name != "Fun" {
		t.Fatalf("fun pile: %+v", doc.Piles)
	}
	doc.FunHours[look.DaySat] = 5400
	if err := b.PutLook(parent, doc); err != nil {
		t.Fatal(err)
	}
	if remaining(t, b, "fun") != 3690 {
		t.Fatalf("put look wrote remaining: %d", remaining(t, b, "fun"))
	}
	again, err := b.Look(parent)
	if err != nil {
		t.Fatal(err)
	}
	if again.FunHours[look.DaySat] != 5400 {
		t.Fatalf("sat hours: %+v", again.FunHours)
	}
	if err := b.PutLook(parent, again); err != nil {
		t.Fatal(err)
	}
	if remaining(t, b, "fun") != 3690 {
		t.Fatal("same-bytes put look must leave remaining")
	}
}

func TestParentLockPersistsAcrossReopen(t *testing.T) {
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.sqlite")
	b, err := Open(path, cfg, afternoon)
	if err != nil {
		t.Fatal(err)
	}
	_, parent, err := b.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetParentLock(parent, true); err != nil {
		t.Fatal(err)
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(path, cfg, afternoon)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = again.Close() })
	if !again.ParentLocked() {
		t.Fatal("parent lock did not persist")
	}
	if remaining(t, again, "fun") != 3600 {
		t.Fatalf("reopen remaining: %d", remaining(t, again, "fun"))
	}
}

func TestPolicyPersistsAcrossReopen(t *testing.T) {
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.sqlite")
	b, err := Open(path, cfg, afternoon)
	if err != nil {
		t.Fatal(err)
	}
	_, parent, err := b.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Grant(parent, "fun", 90, "keep", "keep-policy"); err != nil {
		t.Fatal(err)
	}
	if err := b.SetBedtime(parent, "20:00", "", nil); err != nil {
		t.Fatal(err)
	}
	doc, err := b.Look(parent)
	if err != nil {
		t.Fatal(err)
	}
	doc.FunHours[look.DaySat] = 5400
	if err := b.PutLook(parent, doc); err != nil {
		t.Fatal(err)
	}
	if remaining(t, b, "fun") != 3690 {
		t.Fatalf("policy write refilled: %d", remaining(t, b, "fun"))
	}
	if err := b.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := Open(path, cfg, afternoon)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = again.Close() })
	_, parent, err = again.SeedParent("adam2")
	if err != nil {
		t.Fatal(err)
	}
	st, err := again.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if st.BedtimeStart != "20:00" || st.BedtimeEnd != "07:00" {
		t.Fatalf("bedtime after reopen: %s %s", st.BedtimeStart, st.BedtimeEnd)
	}
	if remaining(t, again, "fun") != 3690 {
		t.Fatalf("remaining after reopen: %d", remaining(t, again, "fun"))
	}
	stored, err := again.Look(parent)
	if err != nil {
		t.Fatal(err)
	}
	if stored.FunHours[look.DaySat] != 5400 {
		t.Fatalf("fun_hours after reopen: %+v", stored.FunHours)
	}
}

func TestCreateAskDuringBedtimeAndLock(t *testing.T) {
	b, parent := openTest(t, bedtime)
	_, askTok, err := b.Mint(parent, MintSpec{Name: "kid-bar", Kind: KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	if !b.BedtimeActive() {
		t.Fatal("fixture should be inside bedtime")
	}
	if _, err := b.CreateAsk(askTok, "fun", 900, "during bedtime"); err != nil {
		t.Fatalf("bank CreateAsk during bedtime: %v", err)
	}
	if err := b.SetParentLock(parent, true); err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateAsk(askTok, "fun", 600, "during lock"); err != nil {
		t.Fatalf("bank CreateAsk during parent lock: %v", err)
	}
	st, err := b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if st.PendingAskCount != 2 {
		t.Fatalf("pending: %d", st.PendingAskCount)
	}
}

func TestBedtimeFromTOMLUntilOverlay(t *testing.T) {
	b, parent := openTest(t, afternoon)
	st, err := b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if st.BedtimeStart != "21:00" || st.BedtimeEnd != "07:00" {
		t.Fatalf("toml bedtime: %s %s", st.BedtimeStart, st.BedtimeEnd)
	}
	if err := b.SetBedtime(parent, "20:00", "", nil); err != nil {
		t.Fatal(err)
	}
	st, err = b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if st.BedtimeStart != "20:00" || st.BedtimeEnd != "07:00" {
		t.Fatalf("overlay bedtime: %s %s", st.BedtimeStart, st.BedtimeEnd)
	}
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func afternoon() time.Time {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return time.Date(2026, 8, 26, 15, 0, 0, 0, loc)
}

func bedtime() time.Time {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return time.Date(2026, 8, 26, 21, 30, 0, 0, loc)
}

func TestReplaceTokenRemints(t *testing.T) {
	b, parent := openTest(t, afternoon)
	first, tok, err := b.ReplaceToken(parent, MintSpec{Name: "kid-bar-read", Kind: KindRead})
	if err != nil || first == "" || tok.Name != "kid-bar-read" {
		t.Fatalf("first: %v %+v", err, tok)
	}
	second, again, err := b.ReplaceToken(parent, MintSpec{Name: "kid-bar-read", Kind: KindRead})
	if err != nil || second == "" || second == first {
		t.Fatalf("replace: %v %s %s", err, first, second)
	}
	if again.ID != tok.ID {
		t.Fatalf("id %s %s", again.ID, tok.ID)
	}
	if _, err := b.LookupSecret(first); err != ErrUnauthorized {
		t.Fatalf("old secret: %v", err)
	}
	if _, err := b.LookupSecret(second); err != nil {
		t.Fatal(err)
	}
}

func TestPairFirstWins(t *testing.T) {
	b, parent := openTest(t, afternoon)
	if parent.Kind != KindParent {
		t.Fatal("bootstrap")
	}
	on, err := b.Paired()
	if err != nil || on {
		t.Fatalf("bootstrap is not a claim: %v %v", on, err)
	}
	secret, tok, err := b.Pair()
	if err != nil || secret == "" || tok == nil || tok.Kind != KindParent || tok.Name != "parent-pair" {
		t.Fatalf("pair: %v %+v", err, tok)
	}
	got, err := b.LookupSecret(secret)
	if err != nil || got.ID != tok.ID {
		t.Fatalf("lookup: %v", err)
	}
	if _, _, err := b.Pair(); err != ErrConflict {
		t.Fatalf("second pair: %v", err)
	}
	on, err = b.Paired()
	if err != nil || !on {
		t.Fatal("claimed")
	}
}

func TestParentPinGrantApproveHoldAndRateLimit(t *testing.T) {
	ts := afternoon()
	clock := func() time.Time { return ts }
	b, parent := openTestClock(t, clock)
	_, askTok, err := b.Mint(parent, MintSpec{Name: "kid-bar", Kind: KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	st, err := b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if st.ParentPinSet || st.Overlay {
		t.Fatal("pin unset")
	}
	if err := b.SetParentPIN(parent, "1234", ""); err != nil {
		t.Fatal(err)
	}
	if err := b.SetParentPIN(askTok, "1234", ""); err != ErrForbidden {
		t.Fatalf("ask set pin: %v", err)
	}
	st, err = b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if !st.ParentPinSet {
		t.Fatal("pin set")
	}
	ask, err := b.CreateAsk(askTok, "fun", 600, "more time")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.PinApprove(askTok, "0000", ask.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("wrong pin: %v", err)
	}
	g, decided, err := b.PinApprove(askTok, "1234", ask.ID)
	if err != nil {
		t.Fatal(err)
	}
	if g.Source != "ask:"+ask.ID || decided.Status != AskApproved {
		t.Fatalf("pin approve: %+v %+v", g, decided)
	}
	if remaining(t, b, "fun") != 4200 {
		t.Fatalf("after approve: %d", remaining(t, b, "fun"))
	}
	if err := b.SetParentLock(parent, true); err != nil {
		t.Fatal(err)
	}
	g, err = b.PinGrant(askTok, "1234", 300)
	if err != nil {
		t.Fatal(err)
	}
	if g.Source != "parent-pin" || g.Seconds != 300 {
		t.Fatalf("pin grant: %+v", g)
	}
	st, err = b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if st.ParentLocked {
		t.Fatal("grant must clear parent lock")
	}
	if !st.BedtimeHold {
		t.Fatal("stay-up hold")
	}
	for i := 0; i < pinFailLimit; i++ {
		if _, err := b.PinGrant(askTok, "9999", 60); !errors.Is(err, ErrForbidden) {
			t.Fatalf("fail %d: %v", i, err)
		}
	}
	if _, err := b.PinGrant(askTok, "1234", 60); !errors.Is(err, ErrForbidden) {
		t.Fatal("cooldown must reject even a good pin")
	}
	ts = ts.Add(31 * time.Second)
	if _, err := b.PinGrant(askTok, "1234", 60); err != nil {
		t.Fatalf("after cooldown: %v", err)
	}
}

func TestOverlayNeedsPinAndHoldSkipsBedtime(t *testing.T) {
	b, parent := openTest(t, bedtime)
	b.Config().BedtimeLock = true
	b.Config().RemoteLock = true
	if b.OverlayActive() {
		t.Fatal("no pin: fail open")
	}
	if err := b.SetParentPIN(parent, "4242", ""); err != nil {
		t.Fatal(err)
	}
	if !b.OverlayActive() {
		t.Fatal("bedtime overlay")
	}
	_, askTok, err := b.Mint(parent, MintSpec{Name: "kid-bar", Kind: KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.PinGrant(askTok, "4242", 600); err != nil {
		t.Fatal(err)
	}
	if b.OverlayActive() {
		t.Fatal("hold skips bedtime overlay")
	}
	st, err := b.Status(parent)
	if err != nil {
		t.Fatal(err)
	}
	if !st.BedtimeHold || st.Overlay {
		t.Fatalf("hold status: %+v", st)
	}
}

func TestLedgerFileIsPrivate(t *testing.T) {
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.sqlite")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Open(path, cfg, afternoon)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("ledger mode %o", st.Mode().Perm())
	}
}

func TestAppMintDefaultsCaps(t *testing.T) {
	b, parent := openTest(t, afternoon)
	_, app, err := b.Mint(parent, MintSpec{Name: "uncapped", Kind: KindApp})
	if err != nil {
		t.Fatal(err)
	}
	if app.MaxSecondsPerGrant != 600 || app.MaxSecondsPerDay != 1800 {
		t.Fatalf("default caps: %+v", app)
	}
	if _, err := b.Grant(app, "fun", 86400, "day", "app-day"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("uncapped day grant: %v", err)
	}
}

func TestLegacyAppTokenCapsApplyAtUse(t *testing.T) {
	b, parent := openTest(t, afternoon)
	secret, app, err := b.Mint(parent, MintSpec{Name: "legacy-app", Kind: KindApp})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.db.Exec(`UPDATE tokens SET max_seconds_per_grant = 0, max_seconds_per_day = 0 WHERE id = ?`, app.ID); err != nil {
		t.Fatal(err)
	}
	got, err := b.LookupSecret(secret)
	if err != nil {
		t.Fatal(err)
	}
	if got.MaxSecondsPerGrant != 600 || got.MaxSecondsPerDay != 1800 {
		t.Fatalf("lookup caps: %+v", got)
	}
	app.MaxSecondsPerGrant = 0
	app.MaxSecondsPerDay = 0
	if _, err := b.Grant(app, "fun", 86400, "legacy", "legacy-day"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("legacy uncapped grant: %v", err)
	}
}

func TestGrantRejectsOverflowSeconds(t *testing.T) {
	b, parent := openTest(t, afternoon)
	if _, err := b.Grant(parent, "fun", 1<<62, "ovf", "ovf"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overflow grant: %v", err)
	}
	if remaining(t, b, "fun") != 3600 {
		t.Fatal("overflow must not credit")
	}
}

func TestCreateAskRejectsHugeSeconds(t *testing.T) {
	b, parent := openTest(t, afternoon)
	_, askTok, err := b.Mint(parent, MintSpec{Name: "kid-bar", Kind: KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.CreateAsk(askTok, "fun", 99999999, "years"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("huge ask: %v", err)
	}
}

func TestPinGrantRejectsHugeSeconds(t *testing.T) {
	b, parent := openTest(t, afternoon)
	if err := b.SetParentPIN(parent, "1234", ""); err != nil {
		t.Fatal(err)
	}
	_, askTok, err := b.Mint(parent, MintSpec{Name: "kid-bar", Kind: KindAsk})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.PinGrant(askTok, "1234", 86400*30); !errors.Is(err, ErrInvalid) {
		t.Fatalf("huge pin grant: %v", err)
	}
}

func openTest(t *testing.T, nowFn func() time.Time) (*Bank, *Token) {
	t.Helper()
	return openTestClock(t, nowFn)
}

func openTestClock(t *testing.T, now func() time.Time) (*Bank, *Token) {
	t.Helper()
	cfg, err := config.ParseFile(packagingPath(t, "config.parent-lab.toml"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ledger.sqlite")
	b, err := Open(path, cfg, now)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	_, parent, err := b.SeedParent("parent")
	if err != nil {
		t.Fatal(err)
	}
	return b, parent
}

func remaining(t *testing.T, b *Bank, group string) int {
	t.Helper()
	n, err := b.Remaining(group)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func grantRow(t *testing.T, b *Bank, key string) (seconds int, source, reason string) {
	t.Helper()
	err := b.db.QueryRow(
		`SELECT seconds, source, reason FROM grants WHERE idempotency_key = ?`,
		key,
	).Scan(&seconds, &source, &reason)
	if err != nil {
		t.Fatal(err)
	}
	return seconds, source, reason
}

func grantCount(t *testing.T, b *Bank, key string) int {
	t.Helper()
	var n int
	err := b.db.QueryRow(`SELECT COUNT(*) FROM grants WHERE idempotency_key = ?`, key).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func spendN(t *testing.T, b *Bank, group string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		res, err := b.Spend(group)
		if err != nil {
			t.Fatal(err)
		}
		if res.Kind != SpendTick {
			t.Fatalf("spend %d of %s: %+v", i, group, res)
		}
	}
}

func packagingPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")
	return filepath.Join(root, "packaging", name)
}
