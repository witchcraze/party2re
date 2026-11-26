package boss_test

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/boss"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

type sealClock struct{ nanos atomic.Int64 }

func (c *sealClock) Now() time.Time      { return time.Unix(0, c.nanos.Load()).UTC() }
func (c *sealClock) Add(d time.Duration) { c.nanos.Add(int64(d)) }
func (c *sealClock) Set(t time.Time)     { c.nanos.Store(t.UnixNano()) }

type sealPenalty struct{}

// sealTamer accepts every capture; a boss defeat may roll one.
type sealTamer struct{}

func (sealTamer) TameMonster(context.Context, string, string, string) error { return nil }

func (sealPenalty) ProjectRunPenalty(context.Context, string, time.Time) error { return nil }

// sealUnsealClock is a fixed unseal timestamp.
type sealUnsealClock struct{ at *time.Time }

func (c sealUnsealClock) GetUnsealedAt(context.Context) (*time.Time, error) { return c.at, nil }

// fixedRNG answers every draw with the same fraction, so an NPC selects its last choice when near 1.
type fixedRNG struct{ v float64 }

func (r fixedRNG) Float64() float64 { return r.v }
func (r fixedRNG) Intn(n int) int   { return min(n-1, int(r.v*float64(n))) }
func (r fixedRNG) IntN(n int) int   { return r.Intn(n) }

// lossyStore reports one committed run transaction as failed (a lost acknowledgement).
type lossyStore struct {
	runhost.Store[boss.Run]
	lose *atomic.Bool
}

func (s lossyStore) WithRunForUpdate(ctx context.Context, id string, create bool, fn func(context.Context) error) error {
	err := s.Store.WithRunForUpdate(ctx, id, create, fn)
	if err == nil && s.lose.CompareAndSwap(true, false) {
		return errors.New("lost acknowledgement")
	}
	return err
}

type sealFixture struct {
	db      *sql.DB
	chars   *database.CharacterRepository
	invs    *database.InventoryRepository
	clock   *sealClock
	lose    atomic.Bool
	banquet atomic.Int32
	news    atomic.Int32
	stage   boss.BossStage
	svc     *boss.RunService
}

// newSealFixture runs one test-owned stage (a unique ID keeps its room slot
// apart on the shared database) whose bosses are weak unless strong is set.
func newSealFixture(t *testing.T, rng random.Generator, bosses ...boss.BossMonster) *sealFixture {
	t.Helper()
	if len(bosses) == 0 {
		bosses = []boss.BossMonster{{Name: "王", HP: 1, Attack: 1, Agility: 1, Crystals: 5, Icon: "mon/190.gif"}}
	}
	return newStageFixture(t, rng, boss.BossStage{Name: "@試練@", LeaderName: bosses[0].Name, Speed: 12, MaxMembers: 6,
		Treasures: runhost.TreasurePools{Items: []string{"item-059"}}, Bosses: bosses})
}

func newStageFixture(t *testing.T, rng random.Generator, stage boss.BossStage) *sealFixture {
	t.Helper()
	f := &sealFixture{db: database.OpenTestDB(t), clock: &sealClock{}}
	t0 := time.Date(2026, 4, 1, 3, 0, 0, 0, time.UTC)
	// The room opens 20 minutes after the unseal, so the clocks tell the multipliers apart.
	f.clock.Set(t0.Add(20 * time.Minute))
	var err error
	if f.chars, err = database.NewCharacterRepository(f.db); err != nil {
		t.Fatal(err)
	}
	if f.invs, err = database.NewInventoryRepository(f.db); err != nil {
		t.Fatal(err)
	}
	store, err := database.NewRunRepository[boss.Run](f.db)
	if err != nil {
		t.Fatal(err)
	}
	equips, err := database.NewEquipmentRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	customs, err := database.NewCustomSkillRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	history, err := database.NewCharacterJobRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(f.db)
	runner, err := economy.NewService(f.chars, f.invs, economy.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := job.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	adapter := battle.NewService(battle.WithCharacterRepository(f.chars), battle.WithCustomSkillRepository(customs), battle.WithInventoryRepository(f.invs), battle.WithDepotRepository(depots), battle.WithEquipmentRepository(equips), battle.WithJobHistoryRepository(history), battle.WithTransactionProvider(tx), battle.WithTransactionRunner(runner), battle.WithMonsterTamer(sealTamer{}))
	stage.ID = "kt-" + id.New()[:12]
	f.stage = stage
	f.svc, err = boss.NewRunService(boss.RunConfig{
		Store: lossyStore{Store: store, lose: &f.lose}, Characters: f.chars, Inventories: f.invs, Builder: adapter, Settler: adapter,
		Jobs: jobs, Penalty: sealPenalty{}, Clock: f.clock, RNG: rng, UnsealClock: sealUnsealClock{at: &t0},
		ExitEffects: func(context.Context, string) error { return nil },
		News: boss.NewsPublisherFunc(func(context.Context, string, string, string, string, time.Time) error {
			f.news.Add(1)
			return nil
		}),
		Stages: []boss.BossStage{f.stage},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.svc.SetBanquet(func(context.Context) error { f.banquet.Add(1); return nil })
	return f
}

func (f *sealFixture) open(t *testing.T) string {
	t.Helper()
	if err := f.svc.OpenSealingRooms(context.Background(), "テスト", []corebattle.LifetimeActor{{Participant: corebattle.Participant{ID: "u", Name: "U"}}}); err != nil {
		t.Fatal(err)
	}
	return f.room(t).ID
}

func (f *sealFixture) room(t *testing.T) *boss.Run {
	t.Helper()
	rooms, err := f.svc.Rooms(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) == 0 {
		return nil
	}
	return &rooms[0]
}

// hero is a strong level-99 character, so defeat rewards never level it up.
func (f *sealFixture) hero(t *testing.T) corecharacter.Character {
	t.Helper()
	ctx := context.Background()
	c, err := database.CreateTestCharacterWithFunds(ctx, f.db, id.New()[:8], 0)
	if err != nil {
		t.Fatal(err)
	}
	c.Level, c.Stats = 99, corecharacter.Stats{HP: 900, MaxHP: 900, MP: 100, MaxMP: 100, Attack: 900, Defense: 900, Agility: 900}
	if err := f.chars.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *sealFixture) join(t *testing.T, c corecharacter.Character, runID string) {
	t.Helper()
	if _, err := f.svc.Join(context.Background(), c.PlayerID, c.ID, runID, id.New()); err != nil {
		t.Fatal(err)
	}
}

func (f *sealFixture) command(c corecharacter.Character, runID, action, target string) (runhost.Receipt, error) {
	f.clock.Add(13 * time.Second)
	run, err := f.svc.FindSlot(context.Background(), "sealing:"+f.stage.ID)
	if err != nil {
		return runhost.Receipt{}, err
	}
	version := 0
	if run != nil {
		version = run.Version
	}
	return f.svc.Command(context.Background(), runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: version, Action: action, TargetID: target})
}

// defeat attacks target until it falls.
func (f *sealFixture) defeat(t *testing.T, c corecharacter.Character, runID, target string) {
	t.Helper()
	for range 30 {
		if a := f.room(t).Actor(target); a == nil || a.HP <= 0 {
			return
		}
		if _, err := f.command(c, runID, "attack", target); err != nil {
			t.Fatal(err)
		}
	}
	t.Fatalf("%s survived", target)
}

func (f *sealFixture) character(t *testing.T, characterID string) corecharacter.Character {
	t.Helper()
	c, err := f.chars.FindByID(context.Background(), characterID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// editRun rewrites the saved room under its run lock.
func (f *sealFixture) editRun(t *testing.T, runID string, edit func(*boss.Run)) {
	t.Helper()
	store, err := database.NewRunRepository[boss.Run](f.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WithRunForUpdate(context.Background(), runID, false, func(ctx context.Context) error {
		r, err := store.FindRun(ctx, runID)
		if err != nil {
			return err
		}
		edit(r)
		r.Version++
		return store.SaveRun(ctx, *r, r.Version-1)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSealingRoomOperationLoop(t *testing.T) {
	f := newSealFixture(t, random.Default())
	ctx := context.Background()
	runID := f.open(t)
	a, late := f.hero(t), f.hero(t)
	f.join(t, a, runID)
	if got := f.character(t, a.ID).Tired; got != a.Tired+20 {
		t.Fatalf("join tired = %d, want %d", got, a.Tired+20)
	}
	before := f.room(t).Version
	if _, err := f.svc.GetActorRun(ctx, a.PlayerID, a.ID); err != nil {
		t.Fatal(err)
	}
	if f.room(t).Version != before {
		t.Fatal("reading the room advanced it")
	}
	if _, err := f.command(a, runID, "seal", ""); !errors.Is(err, boss.ErrEnemiesRemain) {
		t.Fatalf("seal with enemies = %v", err)
	}
	leader := f.room(t).Leader
	f.defeat(t, a, runID, leader)
	run, err := f.svc.GetActorRun(ctx, a.PlayerID, a.ID)
	if err != nil || run == nil {
		t.Fatal(run, err)
	}
	// The boss's 5 crystals doubled 20 minutes after the unseal; the room start would triple them.
	if got := run.Recipient.Applied.Crystals; got != 10 {
		t.Fatalf("crystals = %d, want 10", got)
	}
	if _, err := f.command(a, runID, "seal", ""); err != nil {
		t.Fatal(err)
	}
	r := f.room(t)
	if r.Round != 2 || len(r.Boxes) != 1 || f.character(t, a.ID).HeroCount != a.HeroCount+1 || f.news.Load() != 2 || f.banquet.Load() != 1 {
		t.Fatalf("sealed room: round=%d boxes=%v hero=%d news=%d banquet=%d", r.Round, r.Boxes, f.character(t, a.ID).HeroCount, f.news.Load(), f.banquet.Load())
	}
	if _, err := f.command(a, runID, "seal", ""); !errors.Is(err, boss.ErrSealed) {
		t.Fatalf("second seal = %v", err)
	}
	if _, err := f.svc.Join(ctx, late.PlayerID, late.ID, runID, id.New()); !errors.Is(err, boss.ErrSealed) {
		t.Fatalf("join after sealing = %v", err)
	}
	receipt, err := f.command(a, runID, "chest", r.Boxes[0].ID)
	if err != nil || receipt.Box == nil || receipt.Box.DeliveredTo == "" {
		t.Fatalf("chest = %+v, %v", receipt.Box, err)
	}
	if _, err := f.command(a, runID, "exit", ""); err != nil {
		t.Fatal(err)
	}
	if f.room(t) != nil {
		t.Fatal("the last exit after sealing must delete the room")
	}
}

func TestSealingRoomLastExitKeepsLivingLeader(t *testing.T) {
	f := newSealFixture(t, random.Default(),
		boss.BossMonster{Name: "王", HP: 1, Attack: 1, Agility: 1},
		boss.BossMonster{Name: "壁", HP: 900000, Defense: 900000, Attack: 1, Agility: 1})
	runID := f.open(t)
	a, b := f.hero(t), f.hero(t)
	f.join(t, a, runID)
	if _, err := f.command(a, runID, "exit", ""); err != nil {
		t.Fatal(err)
	}
	r := f.room(t)
	if r == nil || len(r.Roster) != 0 || r.Actor(r.Leader) == nil {
		t.Fatalf("a room with its leader alive must survive its last exit: %+v", r)
	}
	wall := r.Enemies[1]
	f.editRun(t, runID, func(r *boss.Run) { r.Actor(wall).HP = 777 })
	// A later joiner meets the stored enemy state.
	f.join(t, b, runID)
	if got := f.room(t).Actor(wall).HP; got != 777 {
		t.Fatalf("joiner saw wall HP %d", got)
	}
	f.defeat(t, b, runID, f.room(t).Leader)
	if _, err := f.command(b, runID, "exit", ""); err != nil {
		t.Fatal(err)
	}
	if f.room(t) != nil {
		t.Fatal("the last exit deletes the room once the leader is defeated, even before sealing")
	}
}

func TestSealingRoomDejonRemovesFallenPlayers(t *testing.T) {
	// The NPC draws its last choice, デジョン, from こうげき and デジョン.
	f := newSealFixture(t, fixedRNG{v: 0.99}, boss.BossMonster{Name: "王", HP: 900000, Defense: 900000, Attack: 1, Agility: 1})
	ctx := context.Background()
	runID := f.open(t)
	a, b := f.hero(t), f.hero(t)
	f.join(t, a, runID)
	f.join(t, b, runID)
	f.editRun(t, runID, func(r *boss.Run) { r.Actor(a.ID).HP = 0 })
	if _, err := f.command(b, runID, "attack", f.room(t).Leader); err != nil {
		t.Fatal(err)
	}
	r := f.room(t)
	if r.Actor(a.ID) != nil || len(r.Roster) != 1 || r.Recipients[a.ID].Disposition != "dejon" || !r.Retained {
		t.Fatalf("dejon: roster=%v recipient=%+v retained=%v", r.Roster, r.Recipients[a.ID], r.Retained)
	}
	if got := f.character(t, a.ID).Tired; got != a.Tired+50 {
		t.Fatalf("dejon tired = %d, want join 20 + dejon 30", got)
	}
	if run, err := f.svc.GetActorRun(ctx, a.PlayerID, a.ID); err != nil || run != nil {
		t.Fatalf("banished member still enrolled: %+v %v", run, err)
	}
	// The ledger row keeps the banished player's unpaid stock; a rejoin carries it over.
	if _, err := f.db.ExecContext(ctx, `UPDATE run_recipients SET recipient_json=JSON_SET(recipient_json,'$.pending.gold',7) WHERE run_id=? AND character_id=?`, runID, a.ID); err != nil {
		t.Fatal(err)
	}
	f.join(t, a, runID)
	if got := f.room(t).Recipients[a.ID]; got.Disposition != "" || got.Pending.Gold != 7 {
		t.Fatalf("rejoined recipient = %+v", got)
	}
}

func TestSealingRoomStaleAndUnauthorizedCommands(t *testing.T) {
	f := newSealFixture(t, random.Default())
	ctx := context.Background()
	runID := f.open(t)
	a, other := f.hero(t), f.hero(t)
	f.join(t, a, runID)
	version := f.room(t).Version
	f.clock.Add(13 * time.Second)
	if _, err := f.svc.Command(ctx, runhost.Command{PlayerID: a.PlayerID, ActorID: a.ID, RunID: runID, ID: id.New(), Version: version - 1, Action: "seal"}); !errors.Is(err, runhost.ErrConflict) {
		t.Fatalf("stale command = %v", err)
	}
	if _, err := f.svc.Command(ctx, runhost.Command{PlayerID: other.PlayerID, ActorID: a.ID, RunID: runID, ID: id.New(), Version: version, Action: "seal"}); !errors.Is(err, runhost.ErrForbidden) {
		t.Fatalf("another player's actor = %v", err)
	}
	if _, err := f.svc.Join(ctx, other.PlayerID, a.ID, runID, id.New()); !errors.Is(err, runhost.ErrForbidden) {
		t.Fatalf("join as another player's character = %v", err)
	}
	if f.room(t).Version != version {
		t.Fatal("rejected commands changed the room")
	}
}

func TestSealingRoomLostAcknowledgementSealsOnce(t *testing.T) {
	f := newSealFixture(t, random.Default())
	ctx := context.Background()
	runID := f.open(t)
	a := f.hero(t)
	f.join(t, a, runID)
	f.defeat(t, a, runID, f.room(t).Leader)
	f.clock.Add(13 * time.Second)
	cmd := runhost.Command{PlayerID: a.PlayerID, ActorID: a.ID, RunID: runID, ID: id.New(), Version: f.room(t).Version, Action: "seal"}
	f.lose.Store(true)
	first, err := f.svc.Command(ctx, cmd)
	if err != nil {
		t.Fatalf("recovered seal = %v", err)
	}
	replay, err := f.svc.Command(ctx, cmd)
	if err != nil || replay.Version != first.Version {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	// Neither the unconfirmed commit nor the replay celebrates.
	if got := f.character(t, a.ID).HeroCount; got != a.HeroCount+1 || f.banquet.Load() != 0 {
		t.Fatalf("hero count = %d, banquet = %d", got, f.banquet.Load())
	}
}

func TestSealingRoomConcurrentSealStress(t *testing.T) {
	f := newSealFixture(t, random.Default())
	runID := f.open(t)
	heroes := make([]corecharacter.Character, 4)
	for i := range heroes {
		heroes[i] = f.hero(t)
		f.join(t, heroes[i], runID)
	}
	f.defeat(t, heroes[0], runID, f.room(t).Leader)
	f.clock.Add(13 * time.Second)
	version := f.room(t).Version
	var sealed atomic.Int32
	result := testutil.RunConcurrentStressTest(t, testutil.ConcurrencyStressConfig{Workers: len(heroes), OpsPerWorker: 3}, func(worker, _ int) error {
		c := heroes[worker]
		_, err := f.svc.Command(context.Background(), runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: version, Action: "seal"})
		switch {
		case err == nil:
			sealed.Add(1)
		case errors.Is(err, runhost.ErrConflict):
		default:
			return err
		}
		return nil
	})
	if result.Failures != 0 || sealed.Load() != 1 {
		t.Fatalf("failures=%d sealed=%d", result.Failures, sealed.Load())
	}
	for _, c := range heroes {
		if got := f.character(t, c.ID).HeroCount; got != c.HeroCount+1 {
			t.Fatalf("hero count of %s = %d, want %d", c.ID, got, c.HeroCount+1)
		}
	}
	if r := f.room(t); r.Round != 2 || len(r.Boxes) != len(heroes) {
		t.Fatalf("round=%d boxes=%d", r.Round, len(r.Boxes))
	}
}

// Every shipped boss must answer a command: an unsupported job entry in its
// NPC table would reject every command in its room.
func TestSealingCatalogBossesRespond(t *testing.T) {
	stages, err := boss.DefaultBossCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range stages {
		for _, b := range stage.Bosses {
			t.Run(stage.ID+"/"+b.Name, func(t *testing.T) {
				b.HP, b.Defense = 9000000, 9000000
				solo := stage
				solo.LeaderName, solo.Bosses, solo.NeedJoin = b.Name, []boss.BossMonster{b}, ""
				f := newStageFixture(t, random.Default(), solo)
				runID := f.open(t)
				a := f.hero(t)
				f.join(t, a, runID)
				_, err := f.command(a, runID, "defend", "")
				// shortcut: jobs 96 and 101 have no NPC table yet (#2789); drop this once they do.
				if b.OldJob == 96 || b.OldJob == 101 {
					if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
						t.Fatalf("expected the known unsupported NPC job, got %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
