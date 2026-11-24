package challenge_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

type runClock struct{ nanos atomic.Int64 }

func (c *runClock) Now() time.Time      { return time.Unix(0, c.nanos.Load()).UTC() }
func (c *runClock) add(d time.Duration) { c.nanos.Add(int64(d)) }

type runPenalty struct{}

func (runPenalty) ProjectRunPenalty(context.Context, string, time.Time) error { return nil }

// runLobby hands a fixed roster to party admission.
type runLobby struct {
	ids  []string
	tier string
}

func (l runLobby) WithRoster(ctx context.Context, actor, partyID string, fn func(context.Context, runhost.Admission) error) error {
	return fn(ctx, runhost.Admission{Generation: "party:" + partyID, PartyID: partyID, ActorID: actor, CharacterIDs: l.ids, StageID: l.tier, Speed: 3, Name: "trial"})
}
func (runLobby) CleanupLobby(context.Context, runhost.Admission) error { return nil }
func (runLobby) CheckSoloAdmission(context.Context, string) error      { return nil }

type runFixture struct {
	db     *sql.DB
	chars  *database.CharacterRepository
	repo   *database.ChallengeRepository
	store  runhost.Store[challenge.Run]
	clock  *runClock
	tier   string
	cfg    challenge.RunConfig
	svc    *challenge.RunService
	actors []corecharacter.Character
	exits  atomic.Int64
}

// weakTier spawns one undodgeable, uncapturable HP-1 enemy per wave under the
// zero RNG. A negative stage number passes the creator's job-change gate and
// keeps this test's record apart from the shared database's.
func weakTier(tier string) map[string]challenge.ChallengeTier {
	weak := challenge.ChallengeTier{ID: tier, MaxParticipants: 4, Monsters: []challenge.ChallengeMonster{{
		Name: "Test", HP: 2, ExperienceReward: 20, GoldReward: 5,
		Combat: corebattle.LifetimeActor{Participant: corebattle.Participant{Status: corebattle.StatusKinju, Icon: "upload/test"}, Jobs: corebattle.BattleJobs{JobID: "starter", OldJobID: "starter"}},
	}}}
	gated := weak
	gated.ID = "1"
	return map[string]challenge.ChallengeTier{tier: weak, "1": gated}
}

func newRunFixture(t *testing.T, count int) *runFixture {
	t.Helper()
	ctx := context.Background()
	f := &runFixture{db: database.OpenTestDB(t), clock: &runClock{}, tier: fmt.Sprintf("-%d", time.Now().UnixNano())}
	f.clock.nanos.Store(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	var err error
	if f.chars, err = database.NewCharacterRepository(f.db); err != nil {
		t.Fatal(err)
	}
	if f.repo, err = database.NewChallengeRepository(f.db); err != nil {
		t.Fatal(err)
	}
	store, err := database.NewRunRepository[challenge.Run](f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.store = store
	invs, err := database.NewInventoryRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	equips, err := database.NewEquipmentRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(f.db)
	runner, err := economy.NewService(f.chars, invs, economy.WithTransactionProvider(tx))
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
	adapter := battle.NewService(battle.WithCharacterRepository(f.chars), battle.WithCustomSkillRepository(customs), battle.WithInventoryRepository(invs), battle.WithDepotRepository(depots), battle.WithEquipmentRepository(equips), battle.WithJobHistoryRepository(history), battle.WithTransactionProvider(tx), battle.WithTransactionRunner(runner))
	jobs, err := job.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for range count {
		c, err := database.CreateTestCharacterWithFunds(ctx, f.db, id.New()[:8], 100)
		if err != nil {
			t.Fatal(err)
		}
		instance := id.New()
		if _, err := f.db.ExecContext(ctx, `INSERT INTO inventory_items (id,character_id,definition_id,quantity,enhancement_level) VALUES (?,?,'item-105',1,0)`, instance, c.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.ExecContext(ctx, `INSERT INTO equipment_slots (character_id,slot,instance_id) VALUES (?,'accessory',?)`, c.ID, instance); err != nil {
			t.Fatal(err)
		}
		f.actors, ids = append(f.actors, c), append(ids, c.ID)
	}
	f.cfg = challenge.RunConfig{
		Store: f.store, Characters: f.chars, Inventories: invs, Builder: adapter, Settler: adapter, HallOfFame: f.repo,
		Jobs: jobs, Admission: runLobby{ids: ids, tier: f.tier}, Penalty: runPenalty{}, Clock: f.clock, RNG: zeroRNG{}, Tiers: weakTier(f.tier),
		ExitEffects: func(context.Context, string) error { f.exits.Add(1); return nil },
	}
	f.restart(t)
	t.Cleanup(func() {
		_, _ = f.db.ExecContext(context.Background(), `DELETE FROM challenge_hall_of_fame WHERE tier_id = ?`, f.tier) //lint:ignore error-swallow best-effort cleanup of this test's record
	})
	return f
}

// restart rebuilds the service; continuation comes only from saved state.
func (f *runFixture) restart(t *testing.T) {
	t.Helper()
	var err error
	if f.svc, err = challenge.NewRunService(f.cfg); err != nil {
		t.Fatal(err)
	}
}

// stage saves a direct edit of the run, as a crash-free prior command would.
func (f *runFixture) stage(t *testing.T, runID string, edit func(*challenge.Run)) {
	t.Helper()
	r := f.run(t, runID)
	edit(r)
	if err := f.store.WithRunForUpdate(context.Background(), runID, false, func(ctx context.Context) error {
		version := r.Version
		r.Version++
		return f.store.SaveRun(ctx, *r, version)
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *runFixture) run(t *testing.T, runID string) *challenge.Run {
	t.Helper()
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	return r
}

func (f *runFixture) command(t *testing.T, runID string, actor int, action, target string) (runhost.Command, error) {
	t.Helper()
	c := f.actors[actor]
	cmd := runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: f.run(t, runID).Version, Action: action, TargetID: target}
	_, err := f.svc.Command(context.Background(), cmd)
	return cmd, err
}

func (f *runFixture) send(t *testing.T, runID string, actor int, action, target string) runhost.Command {
	t.Helper()
	f.clock.add(3 * time.Second)
	cmd, err := f.command(t, runID, actor, action, target)
	if err != nil {
		t.Fatal(action, err)
	}
	return cmd
}

func (f *runFixture) enterParty(t *testing.T) string {
	t.Helper()
	a := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, id.New())
	if err != nil {
		t.Fatal(err)
	}
	return entry.Command.RunID
}

func (f *runFixture) character(t *testing.T, i int) corecharacter.Character {
	t.Helper()
	c, err := f.chars.FindByID(context.Background(), f.actors[i].ID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestRunWavesWithExplicitCommands(t *testing.T) {
	f := newRunFixture(t, 1)
	ctx := context.Background()
	c := f.actors[0]
	entry, err := f.svc.Enroll(ctx, c.PlayerID, "start", runhost.Admission{ActorID: c.ID, StageID: f.tier, Speed: 3, Name: "trial"})
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	if r := f.run(t, runID); r.Round != 0 || len(r.Enemies) != 0 {
		t.Fatal("start resolved combat", r.Round, r.Enemies)
	}
	if _, err := f.svc.GetActorRun(ctx, c.PlayerID, c.ID); err != nil {
		t.Fatal(err)
	}

	f.send(t, runID, 0, "advance", "")
	r := f.run(t, runID)
	if r.Round != 1 || len(r.Enemies) != 1 || r.Actor(r.Enemies[0]).HP != 1 || r.Actor(c.ID).HP != f.character(t, 0).Stats.HP {
		t.Fatal("advance resolved combat or skipped the spawn", r.Round, r.Enemies)
	}
	enemy := r.Enemies[0]
	if _, err := f.command(t, runID, 0, "attack", enemy); !errors.Is(err, runhost.ErrCooldown) {
		t.Fatal("interval not enforced", err)
	}
	before := *f.run(t, runID)
	f.clock.add(3 * time.Second)
	if _, err := f.command(t, runID, 0, "advance", ""); !errors.Is(err, challenge.ErrEnemiesRemain) {
		t.Fatal(err)
	}
	if after := f.run(t, runID); !reflect.DeepEqual(before, *after) {
		t.Fatal("rejected advance changed the run")
	}

	// A kill pays the shared defeat reward, not a wave total.
	attack := f.send(t, runID, 0, "attack", enemy)
	if paid := f.character(t, 0); paid.Money != c.Money+5 {
		t.Fatal("kill reward", paid.Money)
	}
	f.restart(t)
	replay, err := f.svc.Command(ctx, attack)
	if err != nil || f.character(t, 0).Money != c.Money+5 {
		t.Fatal("receipt replay paid again", replay, err)
	}
	foreign := runhost.Command{PlayerID: "someone-else", ActorID: c.ID, RunID: runID, ID: id.New(), Version: f.run(t, runID).Version, Action: "advance"}
	if _, err := f.svc.Command(ctx, foreign); !errors.Is(err, runhost.ErrForbidden) {
		t.Fatal("foreign command", err)
	}

	// HP carries into the next wave with no recovery.
	f.stage(t, runID, func(r *challenge.Run) { r.Actor(c.ID).HP = 5 })
	f.send(t, runID, 0, "advance", "")
	hof, err := f.repo.GetHallOfFame(ctx, f.tier)
	if err != nil || hof == nil || hof.HighestRound != 1 || hof.PartyName != "trial" || len(hof.Members) != 1 {
		t.Fatal("record not raised once the first wave was cleared", hof, err)
	}
	if r := f.run(t, runID); r.Round != 2 || r.Record != 1 || r.Actor(c.ID).HP != 5 {
		t.Fatal(r.Round, r.Record, r.Actor(c.ID).HP)
	}

	stayed := f.character(t, 0)
	exit := f.send(t, runID, 0, "exit", "")
	left := f.character(t, 0)
	if left.Tired != stayed.Tired+2*3+1 || left.Experience != stayed.Experience+2*20 || left.Stats.HP != 5 || f.exits.Load() != 1 {
		t.Fatal("exit effects", left.Tired, left.Experience, f.exits.Load())
	}
	if _, err := f.svc.Command(ctx, exit); err != nil || f.character(t, 0).Tired != left.Tired || f.exits.Load() != 1 {
		t.Fatal("exit replay settled twice", err)
	}
}

func TestRunFirstAdvanceIsLeaderOnlyAcrossHandoff(t *testing.T) {
	f := newRunFixture(t, 2)
	runID := f.enterParty(t)
	f.clock.add(3 * time.Second)
	if _, err := f.command(t, runID, 1, "advance", ""); !errors.Is(err, challenge.ErrLeaderOnly) {
		t.Fatal(err)
	}
	f.send(t, runID, 0, "exit", "")
	f.send(t, runID, 1, "advance", "")
	if r := f.run(t, runID); r.Round != 1 || r.LeaderID != f.actors[1].ID {
		t.Fatal(r.Round, r.LeaderID)
	}
}

// An all-fallen room stays open; each member leaves with the exit effects (#2656).
func TestRunAllFallenRoomStaysOpenForEachExit(t *testing.T) {
	f := newRunFixture(t, 2)
	ctx := context.Background()
	runID := f.enterParty(t)
	f.send(t, runID, 0, "advance", "")
	f.wipe(t, runID)
	f.clock.add(3 * time.Second)
	if _, err := f.command(t, runID, 0, "advance", ""); !errors.Is(err, challenge.ErrActorFallen) {
		t.Fatal(err)
	}
	for i := range f.actors {
		fallen := f.character(t, i)
		exit := f.send(t, runID, i, "exit", "")
		if _, err := f.svc.Command(ctx, exit); err != nil {
			t.Fatal(err)
		}
		c := f.character(t, i)
		if c.Tired != fallen.Tired+1*3+1 || c.Experience != fallen.Experience+1*20 {
			t.Fatal("fallen exit effects", i, c.Tired, c.Experience)
		}
	}
	if f.exits.Load() != 2 {
		t.Fatal("exit effects ran", f.exits.Load())
	}
}

func (f *runFixture) wipe(t *testing.T, runID string) {
	t.Helper()
	f.stage(t, runID, func(r *challenge.Run) {
		for _, c := range f.actors {
			r.Actor(c.ID).HP = 0
		}
	})
}

// Idle cleanup of an all-fallen room follows the shared room lifecycle (#2656).
func TestRunAllFallenRoomIdleCleanup(t *testing.T) {
	f := newRunFixture(t, 2)
	runID := f.enterParty(t)
	f.send(t, runID, 0, "advance", "")
	f.wipe(t, runID)
	f.clock.add(1801 * time.Second)
	f.send(t, runID, 1, "cleanup", "")
	if r, err := f.store.FindRun(context.Background(), runID); err != nil || r != nil || f.exits.Load() != 0 {
		t.Fatal("cleanup left the room or ran exit effects", r, err, f.exits.Load())
	}
	for _, c := range f.actors {
		p, err := f.svc.GetActorPenalty(context.Background(), c.PlayerID, c.ID)
		if err != nil || p == nil || p.Recipient.Disposition != "timeout" || p.Recipient.PenaltyUntil.IsZero() {
			t.Fatal(c.ID, p, err)
		}
	}
}

// Competing advances on one version apply once and spawn one wave.
func TestRunPairedAdvanceAppliesOnce(t *testing.T) {
	f := newRunFixture(t, 2)
	runID := f.enterParty(t)
	f.send(t, runID, 0, "advance", "")
	enemy := f.run(t, runID).Enemies[0]
	f.send(t, runID, 0, "attack", enemy)
	f.clock.add(3 * time.Second)
	version := f.run(t, runID).Version
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		c := f.actors[(worker+op)%2]
		_, err := f.svc.Command(context.Background(), runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: "advance-" + c.ID, Version: version, Action: "advance"})
		if errors.Is(err, runhost.ErrConflict) {
			return nil
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	r := f.run(t, runID)
	if r.Round != 2 || len(r.Enemies) != 1 || r.Version != version+1 {
		t.Fatal("advance applied more than once", r.Round, r.Enemies, r.Version)
	}
	// The other member's share waits in its ledger until its own command.
	total := 0
	for i, c := range f.actors {
		total += f.character(t, i).Money + r.Recipients[c.ID].Pending.Gold
	}
	if total != 2*100+2*5 {
		t.Fatal("assets not conserved", total)
	}
}

// Only the creator meets the stage's job-change and level gates; every member
// meets the stocked level-up check (quest.cgi:539–540,712,971).
func TestRunAdmissionGates(t *testing.T) {
	f := newRunFixture(t, 2)
	ctx := context.Background()
	set := func(i, level, exp int) {
		t.Helper()
		if _, err := f.db.ExecContext(ctx, `UPDATE characters SET level = ?, experience = ? WHERE id = ?`, level, exp, f.actors[i].ID); err != nil {
			t.Fatal(err)
		}
	}
	a, b := f.actors[0], f.actors[1]
	if _, err := f.svc.Enroll(ctx, a.PlayerID, "stage-1", runhost.Admission{ActorID: a.ID, StageID: "1", Speed: 3}); !errors.Is(err, challenge.ErrJobLevelTooLow) {
		t.Fatal(err)
	}
	set(1, 1, 10)
	if _, err := f.svc.EnterParty(ctx, a.PlayerID, a.ID, id.New()); !errors.Is(err, challenge.ErrPendingLevelUp) {
		t.Fatal(err)
	}
	set(1, 100, 0)
	if _, err := f.svc.Enroll(ctx, b.PlayerID, "creator", runhost.Admission{ActorID: b.ID, StageID: f.tier, Speed: 3}); !errors.Is(err, challenge.ErrLevelTooHigh) {
		t.Fatal(err)
	}
	if _, err := f.svc.EnterParty(ctx, a.PlayerID, a.ID, id.New()); err != nil {
		t.Fatal("a level-100 joiner was rejected", err)
	}
}

// A concurrent lower record never overwrites a higher one (update_record's recheck).
func TestRaiseHallOfFameKeepsTheHigherRound(t *testing.T) {
	f := newRunFixture(t, 0)
	ctx := context.Background()
	for _, step := range []struct {
		round  int
		name   string
		raised bool
		want   int
	}{{3, "first", true, 3}, {2, "lower", false, 3}, {3, "equal", false, 3}, {4, "higher", true, 4}} {
		raised, err := f.repo.RaiseHallOfFame(ctx, challenge.HallOfFameEntry{TierID: f.tier, HighestRound: step.round, PartyName: step.name, ClearedAt: f.clock.Now()})
		if err != nil || raised != step.raised {
			t.Fatal(step.name, raised, err)
		}
		hof, err := f.repo.GetHallOfFame(ctx, f.tier)
		if err != nil || hof.HighestRound != step.want || (step.raised && hof.PartyName != step.name) {
			t.Fatal(step.name, hof, err)
		}
	}
}
