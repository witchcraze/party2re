package database_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

const presenceBase = int64(1_700_000_000)

type presenceFixture struct {
	db    *sql.DB
	repo  *database.PresenceRepository
	svc   *character.PresenceService
	clock atomic.Int64 // seconds after presenceBase
}

// Observation deletes expired rows table-wide, so every test owns a dedicated DB.
func newPresenceFixture(t *testing.T) *presenceFixture {
	t.Helper()
	db := isolatedRepositoryDB(t, "players", "characters", "character_depots", "depot_items", "guilds", "guild_members", "character_profiles", "character_presence_snapshots")
	repo, err := database.NewPresenceRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f := &presenceFixture{db: db, repo: repo}
	f.svc = f.service(t, repo)
	return f
}

func (f *presenceFixture) service(t *testing.T, repo character.PresenceRepository) *character.PresenceService {
	t.Helper()
	svc, err := character.NewPresenceService(repo, database.NewTransactionProvider(f.db), func() time.Time {
		return time.Unix(presenceBase+f.clock.Load(), 0)
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func (f *presenceFixture) character(t *testing.T, name string) corecharacter.Character {
	t.Helper()
	c, err := database.CreateTestCharacter(context.Background(), f.db, name)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *presenceFixture) at(clock int64) *presenceFixture { f.clock.Store(clock); return f }

func (f *presenceFixture) save(t *testing.T, c corecharacter.Character) {
	t.Helper()
	if err := f.svc.RecordSave(context.Background(), c.PlayerID, c.ID); err != nil {
		t.Fatal(err)
	}
}

func (f *presenceFixture) enter(t *testing.T, c corecharacter.Character, msg *string) {
	t.Helper()
	if err := f.svc.Enter(context.Background(), c.PlayerID, c.ID, msg); err != nil {
		t.Fatal(err)
	}
}

func (f *presenceFixture) observe(t *testing.T) []character.PresenceSnapshot {
	t.Helper()
	obs, err := f.svc.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if obs.OnlineCount != len(obs.Characters) || obs.ObservedAt.Unix() != presenceBase+f.clock.Load() {
		t.Fatalf("inconsistent observation %+v", obs)
	}
	return obs.Characters
}

func (f *presenceFixture) sampledAt(t *testing.T, c corecharacter.Character) int64 {
	t.Helper()
	st, err := f.svc.State(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if st.SampledAt.IsZero() {
		return -1
	}
	return st.SampledAt.Unix() - presenceBase
}

func (f *presenceFixture) snapshotCount(t *testing.T, c corecharacter.Character) int {
	t.Helper()
	var n int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM character_presence_snapshots WHERE character_id = ?`, c.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func ids(rows []character.PresenceSnapshot) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.CharacterID
	}
	return out
}

func sameIDs(t *testing.T, got []character.PresenceSnapshot, want ...corecharacter.Character) {
	t.Helper()
	g := ids(got)
	if len(g) != len(want) {
		t.Fatalf("got %v, want %d rows", g, len(want))
	}
	for i, c := range want {
		if g[i] != c.ID {
			t.Fatalf("row %d = %s, want %s (all %v)", i, g[i], c.ID, g)
		}
	}
}

func TestPresenceSamplingWindowAndExpiry(t *testing.T) {
	f := newPresenceFixture(t)
	a := f.character(t, "PresA")
	if f.sampledAt(t, a) != -1 || len(f.observe(t)) != 0 {
		t.Fatal("no record must mean offline")
	}
	f.at(0).save(t, a)
	for _, clock := range []int64{899, 900} {
		f.at(clock).save(t, a)
		if got := f.sampledAt(t, a); got != 0 {
			t.Fatalf("save at %d moved sample clock to %d", clock, got)
		}
	}
	f.at(901).save(t, a)
	if got := f.sampledAt(t, a); got != 901 {
		t.Fatalf("sample clock = %d, want 901", got)
	}

	b := f.character(t, "PresB")
	f.at(2000).save(t, b)
	for _, clock := range []int64{2899, 2900} {
		sameIDs(t, f.at(clock).observe(t), b)
	}
	if rows := f.at(2901).observe(t); len(rows) != 0 {
		t.Fatalf("expired sample still visible: %v", ids(rows))
	}
}

func TestPresenceEntryHidesNewerSample(t *testing.T) {
	for _, observeAt1000 := range []bool{true, false} {
		f := newPresenceFixture(t)
		c := f.character(t, "PresE")
		f.at(0).save(t, c)
		f.at(500).enter(t, c, nil)
		if got := f.sampledAt(t, c); got != 0 {
			t.Fatalf("entry reset sample clock to %d", got)
		}
		f.at(901).save(t, c)
		if observeAt1000 {
			sameIDs(t, f.at(1000).observe(t), c)
			if f.snapshotCount(t, c) != 1 {
				t.Fatal("hidden sample must be discarded by observation")
			}
			sameIDs(t, f.at(1400).observe(t), c)
			if rows := f.at(1401).observe(t); len(rows) != 0 {
				t.Fatalf("discarded sample revived: %v", ids(rows))
			}
		} else {
			// Without the intermediate observation the 901 sample is still valid.
			sameIDs(t, f.at(1401).observe(t), c)
		}
		if got := f.sampledAt(t, c); got != 901 {
			t.Fatalf("sample clock = %d, want 901", got)
		}
	}
}

func TestPresenceDisplayOrder(t *testing.T) {
	f := newPresenceFixture(t)
	a, b, c, d, e := f.character(t, "OrdA"), f.character(t, "OrdB"), f.character(t, "OrdC"), f.character(t, "OrdD"), f.character(t, "OrdE")
	// Samples (first save) oldest first, equal time by publication order.
	f.at(10).save(t, c)
	f.at(10).save(t, d)
	f.at(5).save(t, e)
	// Entries newest first; equal time puts the later publication first. Saving
	// before entry keeps the entry's due sample out of the sample list.
	f.at(0).save(t, a)
	f.at(0).save(t, b)
	f.at(10).enter(t, a, nil)
	f.at(10).enter(t, b, nil)
	sameIDs(t, f.at(20).observe(t), b, a, e, c, d)
}

func TestPresenceMessageAndSnapshotFields(t *testing.T) {
	f := newPresenceFixture(t)
	c := f.character(t, "PresM")
	ctx := context.Background()
	msg := ` <b>"Hi" & 'yo'</b> cafe` + "́ 　"
	f.at(0).enter(t, c, &msg)
	f.at(1).enter(t, c, nil)
	st, err := f.svc.State(ctx, c.ID)
	if err != nil || st.LoginMessage != msg {
		t.Fatalf("nil must preserve message: %q %v", st.LoginMessage, err)
	}
	rows := f.observe(t)
	raw, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var back []character.PresenceSnapshot
	if err := json.Unmarshal(raw, &back); err != nil || back[0].LoginMessage != msg || back[0].Name != c.Name {
		t.Fatalf("round trip lost text: %+v %v", back, err)
	}

	// Snapshot is immutable: later rename/profile/message changes do not alter it.
	if _, err := f.db.Exec(`UPDATE characters SET name = 'Renamed', color = '#000000' WHERE id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	empty := ""
	if err := f.svc.Enter(ctx, "someone-else", c.ID, &empty); !errors.Is(err, character.ErrForbidden) {
		t.Fatalf("foreign player entry: %v", err)
	}
	bad := "line\nbreak"
	if err := f.svc.Enter(ctx, c.PlayerID, c.ID, &bad); !errors.Is(err, character.ErrInvalidLoginMessage) {
		t.Fatalf("invalid message: %v", err)
	}
	got := f.at(2).observe(t)
	if got[0].Name != c.Name || got[0].LoginMessage != msg || got[0].CharacterID != c.ID {
		t.Fatalf("snapshot changed: %+v", got[0])
	}
	if st, _ := f.svc.State(ctx, c.ID); st.LoginMessage != msg {
		t.Fatal("rejected entry changed message")
	}

	f.at(3).enter(t, c, &empty)
	got = f.observe(t)
	if got[0].LoginMessage != "" || got[0].Name != "Renamed" || got[0].CharacterID != c.ID {
		t.Fatalf("empty must clear and new entry must copy current fields: %+v", got[0])
	}
}

func TestPresenceProfileFieldsAndGuild(t *testing.T) {
	f := newPresenceFixture(t)
	ctx := context.Background()
	g, c, err := database.CreateTestGuildWithLeader(ctx, f.db, "PresGuild", 0)
	if err != nil {
		t.Fatal(err)
	}
	guildID := g.ID
	if _, err := f.db.Exec(`INSERT INTO character_profiles (character_id, avatar_url, aura_effect) VALUES (?, 'data:image/svg+xml;base64,PHN2Zz4=', 3)`, c.ID); err != nil {
		t.Fatal(err)
	}
	f.at(0).enter(t, c, nil)
	row := f.observe(t)[0]
	if row.GuildID != guildID || row.GuildName != g.Name || row.AuraEffect != 3 || row.IconURL != "" {
		t.Fatalf("unexpected public fields: %+v", row)
	}
	if _, err := f.db.Exec(`UPDATE character_profiles SET avatar_url = 'https://i.imgur.com/AvHTest.gif' WHERE character_id = ?`, c.ID); err != nil {
		t.Fatal(err)
	}
	f.at(1).enter(t, c, nil)
	if row := f.observe(t)[0]; row.IconURL != "https://i.imgur.com/AvHTest.gif" {
		t.Fatalf("safe icon not published: %+v", row)
	}
}

func TestPresenceUnsafeStoredDataFailsObservation(t *testing.T) {
	for _, tamper := range []string{
		`UPDATE character_presence_snapshots SET login_message = 'a\nb'`,
		`UPDATE character_presence_snapshots SET icon_url = 'javascript:alert(1)'`,
		`UPDATE character_presence_snapshots SET color = 'red;x'`,
	} {
		f := newPresenceFixture(t)
		ok, bad := f.character(t, "SafeOK"), f.character(t, "SafeBad")
		f.at(0).enter(t, ok, nil)
		f.at(0).enter(t, bad, nil)
		if _, err := f.db.Exec(tamper+` WHERE character_id = ?`, bad.ID); err != nil {
			t.Fatal(err)
		}
		if obs, err := f.svc.Observe(context.Background()); !errors.Is(err, character.ErrUnsafePresenceData) || obs.Characters != nil {
			t.Fatalf("%s: got %+v, %v", tamper, obs, err)
		}
	}
}

func TestPresenceBanDeleteAndSiblings(t *testing.T) {
	f := newPresenceFixture(t)
	ctx := context.Background()
	a := f.character(t, "SibA")
	sib, err := corecharacter.New("SibB")
	if err != nil {
		t.Fatal(err)
	}
	sib.PlayerID = a.PlayerID
	charRepo, err := database.NewCharacterRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := charRepo.Save(ctx, sib); err != nil {
		t.Fatal(err)
	}
	other := f.character(t, "SibC")
	f.at(0).enter(t, a, nil)
	f.at(0).enter(t, a, nil) // a second session for the same character
	f.at(0).enter(t, sib, nil)
	f.at(0).enter(t, other, nil)
	if rows := f.observe(t); len(rows) != 3 {
		t.Fatalf("want three distinct characters, got %v", ids(rows))
	}

	if _, err := f.db.Exec(`UPDATE players SET banned_at = NOW(6) WHERE id = ?`, a.PlayerID); err != nil {
		t.Fatal(err)
	}
	sameIDs(t, f.at(1).observe(t), other)
	if _, err := f.db.Exec(`UPDATE players SET banned_at = NULL WHERE id = ?`, a.PlayerID); err != nil {
		t.Fatal(err)
	}
	sameIDs(t, f.at(2).observe(t), other) // unban records no activity

	f.at(3).enter(t, a, nil)
	f.at(3).enter(t, sib, nil)
	for _, q := range []string{`DELETE FROM depot_items WHERE character_id = ?`, `DELETE FROM character_depots WHERE character_id = ?`, `DELETE FROM characters WHERE id = ?`} {
		if _, err := f.db.Exec(q, a.ID); err != nil {
			t.Fatal(err)
		}
	}
	if f.snapshotCount(t, a) != 0 {
		t.Fatal("deleted character kept presence rows")
	}
	sameIDs(t, f.observe(t), sib, other) // the sibling's evidence survives
}

type failingPresenceRepo struct {
	character.PresenceRepository
	failOn string
}

var errInjected = errors.New("injected failure")

func (r failingPresenceRepo) SetLoginMessage(ctx context.Context, id, msg string) error {
	if r.failOn == "message" {
		return errInjected
	}
	return r.PresenceRepository.SetLoginMessage(ctx, id, msg)
}

func (r failingPresenceRepo) AdvanceSampleClock(ctx context.Context, id string, now, window int64) (bool, error) {
	if r.failOn == "clock" {
		return false, errInjected
	}
	return r.PresenceRepository.AdvanceSampleClock(ctx, id, now, window)
}

func (r failingPresenceRepo) PublishPresence(ctx context.Context, id string, kind character.PresenceKind, at int64, icon string) error {
	if r.failOn == string(kind) {
		return errInjected
	}
	return r.PresenceRepository.PublishPresence(ctx, id, kind, at, icon)
}

func (r failingPresenceRepo) ObservePresence(ctx context.Context, now, window int64) ([]character.PresenceSnapshot, error) {
	if r.failOn == "observe" {
		return nil, errInjected
	}
	return r.PresenceRepository.ObservePresence(ctx, now, window)
}

func TestPresenceFailuresRollBackAsUnit(t *testing.T) {
	for _, failOn := range []string{"message", "clock", "sample", "entry", "caller"} {
		f := newPresenceFixture(t)
		c := f.character(t, "RbChar")
		svc := f.service(t, failingPresenceRepo{f.repo, failOn})
		msg := "new"
		ctx := context.Background()
		err := database.NewTransactionProvider(f.db).RunInTx(ctx, func(ctx context.Context) error {
			if err := svc.Enter(ctx, c.PlayerID, c.ID, &msg); err != nil {
				return err
			}
			return errInjected // the caller's save fails after entry joined it
		})
		if !errors.Is(err, errInjected) {
			t.Fatalf("%s: %v", failOn, err)
		}
		st, err := f.svc.State(ctx, c.ID)
		if err != nil || st.LoginMessage != "" || !st.SampledAt.IsZero() || f.snapshotCount(t, c) != 0 {
			t.Fatalf("%s: partial commit %+v snapshots=%d", failOn, st, f.snapshotCount(t, c))
		}
	}
	f := newPresenceFixture(t)
	if _, err := f.service(t, failingPresenceRepo{f.repo, "observe"}).Observe(context.Background()); !errors.Is(err, errInjected) {
		t.Fatalf("observation failure hidden: %v", err)
	}
}

type countingPresenceRepo struct {
	character.PresenceRepository
	samples *atomic.Int64
}

func (r countingPresenceRepo) PublishPresence(ctx context.Context, id string, kind character.PresenceKind, at int64, icon string) error {
	if kind == character.PresenceSample {
		r.samples.Add(1)
	}
	return r.PresenceRepository.PublishPresence(ctx, id, kind, at, icon)
}

func TestPresenceConcurrentSavesPublishOneSample(t *testing.T) {
	f := newPresenceFixture(t)
	c := f.character(t, "RaceChar")
	var samples atomic.Int64
	f.svc = f.service(t, countingPresenceRepo{f.repo, &samples})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f.at(1000)
	var ops []func() error
	for range 8 {
		ops = append(ops, func() error { return f.svc.RecordSave(ctx, c.PlayerID, c.ID) })
	}
	for range 2 {
		ops = append(ops, func() error { return f.svc.Enter(ctx, c.PlayerID, c.ID, nil) })
		ops = append(ops, func() error { _, err := f.svc.Observe(ctx); return err })
	}
	for _, err := range testutil.RunRace(ops...) {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := f.sampledAt(t, c); got != 1000 || samples.Load() != 1 {
		t.Fatalf("sample clock = %d, sample publications = %d", got, samples.Load())
	}
	sameIDs(t, f.observe(t), c)
	f.at(500).save(t, c) // an older clock reading never moves the clock backwards
	if got := f.sampledAt(t, c); got != 1000 {
		t.Fatalf("clock moved backwards to %d", got)
	}
}

func (f *presenceFixture) recordedAt(t *testing.T, c corecharacter.Character, kind character.PresenceKind) int64 {
	t.Helper()
	var at int64
	if err := f.db.QueryRow(`SELECT recorded_at FROM character_presence_snapshots WHERE character_id = ? AND kind = ?`, c.ID, string(kind)).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at - presenceBase
}

func TestPresenceConcurrentEntriesNeverRegress(t *testing.T) {
	f := newPresenceFixture(t)
	c := f.character(t, "EntRace")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// An entry that started at t=10 but waits for the character lock is recorded
	// at the time it acquires the lock, not when it was requested.
	holder, err := f.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.ExecContext(ctx, `SELECT id FROM characters WHERE id = ? FOR UPDATE`, c.ID); err != nil {
		t.Fatal(err)
	}
	f.at(10)
	locking := make(chan struct{})
	svc := f.service(t, lockSignalRepo{f.repo, locking})
	done := make(chan error, 1)
	go func() { done <- svc.Enter(ctx, c.PlayerID, c.ID, nil) }()
	select {
	case <-locking: // requested at t=10; the lock is still held, so the call is blocked
	case <-ctx.Done():
		t.Fatal("entry never requested the character lock")
	}
	f.at(20)
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := f.recordedAt(t, c, character.PresenceEntry); got != 20 {
		t.Fatalf("entry recorded at %d, want 20", got)
	}

	// Another instance with a lagging clock cannot replace the newer snapshot.
	if err := f.repo.PublishPresence(ctx, c.ID, character.PresenceEntry, presenceBase+15, ""); err != nil {
		t.Fatal(err)
	}
	if got := f.recordedAt(t, c, character.PresenceEntry); got != 20 {
		t.Fatalf("older publication regressed entry to %d", got)
	}
	seq := func() (n int64) {
		if err := f.db.QueryRow(`SELECT seq FROM character_presence_snapshots WHERE character_id = ? AND kind = 'entry'`, c.ID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := seq()
	if err := f.repo.PublishPresence(ctx, c.ID, character.PresenceEntry, presenceBase+20, ""); err != nil {
		t.Fatal(err)
	}
	if got := f.recordedAt(t, c, character.PresenceEntry); got != 20 || seq() <= before {
		t.Fatal("equal-time republication must replace the entry and take a later publication order")
	}
}

// lockSignalRepo reports when an operation is about to request the character lock.
type lockSignalRepo struct {
	character.PresenceRepository
	locking chan struct{}
}

func (r lockSignalRepo) LockPresenceOwner(ctx context.Context, id string) (string, error) {
	close(r.locking)
	return r.PresenceRepository.LockPresenceOwner(ctx, id)
}

func TestPresenceSetLoginMessagePublishesNothing(t *testing.T) {
	f := newPresenceFixture(t)
	c := f.character(t, "MsgOnly")
	ctx := context.Background()
	if err := f.svc.SetLoginMessage(ctx, "someone-else", c.ID, "x"); !errors.Is(err, character.ErrForbidden) {
		t.Fatalf("foreign player: %v", err)
	}
	if err := f.svc.SetLoginMessage(ctx, c.PlayerID, c.ID, "a\nb"); !errors.Is(err, character.ErrInvalidLoginMessage) {
		t.Fatalf("invalid message: %v", err)
	}
	if err := f.svc.SetLoginMessage(ctx, c.PlayerID, c.ID, "性格：ムッツリスケベ"); err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.State(ctx, c.ID)
	if err != nil || st.LoginMessage != "性格：ムッツリスケベ" || !st.SampledAt.IsZero() || f.snapshotCount(t, c) != 0 {
		t.Fatalf("message-only update: %+v snapshots=%d err=%v", st, f.snapshotCount(t, c), err)
	}
}
