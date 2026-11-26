package pvp_test

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

type runClock struct{ nanos atomic.Int64 }

func (c *runClock) Now() time.Time      { return time.Unix(0, c.nanos.Load()).UTC() }
func (c *runClock) add(d time.Duration) { c.nanos.Add(int64(d)) }

type runPenalty struct{}

func (runPenalty) ProjectRunPenalty(context.Context, string, time.Time) error { return nil }

type zeroRNG struct{}

func (zeroRNG) Intn(int) int     { return 0 }
func (zeroRNG) IntN(int) int     { return 0 }
func (zeroRNG) Float64() float64 { return 0 }

const runBet = 100

type matchFixture struct {
	db     *sql.DB
	chars  *database.CharacterRepository
	claims *database.PvPTransitionRepository
	rooms  *pvp.MemoryRoomRepository
	store  runhost.Store[pvp.Run]
	clock  *runClock
	lobby  *pvp.Service
	cfg    pvp.RunConfig
	svc    *pvp.RunService
	actors []corecharacter.Character
	roomID string
	exits  atomic.Int64
}

// newMatchFixture opens a lobby room with one bet per member; colors[i] is
// member i's team color, "" leaving it unset.
func newMatchFixture(t *testing.T, targetWins int, colors ...string) *matchFixture {
	t.Helper()
	ctx := context.Background()
	f := &matchFixture{db: database.OpenTestDB(t), clock: &runClock{}, rooms: pvp.NewMemoryRoomRepository()}
	f.clock.nanos.Store(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).UnixNano())
	var err error
	if f.chars, err = database.NewCharacterRepository(f.db); err != nil {
		t.Fatal(err)
	}
	if f.claims, err = database.NewPvPTransitionRepository(f.db); err != nil {
		t.Fatal(err)
	}
	store, err := database.NewRunRepository[pvp.Run](f.db)
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
	prizes := newSQLPrizeRemitter(t, f.db)
	if f.lobby, err = pvp.NewService(f.rooms, f.chars, mockBattleEngine{}, pvp.WithTransactionProvider(tx), pvp.WithPrizeRemitter(prizes), pvp.WithTransitionRepository(f.claims), pvp.WithClock(f.clock.Now)); err != nil {
		t.Fatal(err)
	}
	for range colors {
		c, err := database.CreateTestCharacterWithFunds(ctx, f.db, id.New()[:8], 1000)
		if err != nil {
			t.Fatal(err)
		}
		f.actors = append(f.actors, c)
	}
	room, err := f.lobby.CreateRoom(ctx, f.actors[0].ID, pvp.CreateRoomRequest{Name: "arena", Speed: 3, Bet: runBet, MaxMembers: len(colors), TargetWins: targetWins})
	if err != nil {
		t.Fatal(err)
	}
	f.roomID = room.Room.ID
	for i, c := range f.actors {
		if i > 0 {
			if _, err := f.lobby.JoinRoom(ctx, c.ID, f.roomID, ""); err != nil {
				t.Fatal(err)
			}
		}
		if colors[i] != "" {
			if _, err := f.lobby.SelectTeam(ctx, c.ID, f.roomID, colors[i]); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.cfg = pvp.RunConfig{
		Store: f.store, Characters: f.chars, Inventories: invs, Builder: adapter, Settler: adapter, Jobs: jobs,
		Lobby: f.lobby, Prizes: prizes, Penalty: runPenalty{}, Clock: f.clock, RNG: zeroRNG{},
		ExitEffects: func(context.Context, string) error { f.exits.Add(1); return nil },
	}
	f.restart(t)
	return f
}

// restart rebuilds the service; continuation comes only from saved state.
func (f *matchFixture) restart(t *testing.T) {
	t.Helper()
	var err error
	if f.svc, err = pvp.NewRunService(f.cfg); err != nil {
		t.Fatal(err)
	}
}

func (f *matchFixture) start(t *testing.T, actor int) (runhost.Receipt, error) {
	t.Helper()
	c := f.actors[actor]
	return f.svc.EnterParty(context.Background(), c.PlayerID, c.ID, f.roomID)
}

func (f *matchFixture) run(t *testing.T, runID string) *pvp.Run {
	t.Helper()
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	return r
}

// stage saves a direct edit of the run, as a crash-free prior command would.
func (f *matchFixture) stage(t *testing.T, runID string, edit func(*pvp.Run)) {
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

func (f *matchFixture) command(t *testing.T, runID string, actor int, action, target string) (runhost.Command, error) {
	t.Helper()
	c := f.actors[actor]
	cmd := runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: runID, ID: id.New(), Version: f.run(t, runID).Version, Action: action, TargetID: target}
	_, err := f.svc.Command(context.Background(), cmd)
	return cmd, err
}

func (f *matchFixture) send(t *testing.T, runID string, actor int, action, target string) runhost.Command {
	t.Helper()
	f.clock.add(3 * time.Second)
	cmd, err := f.command(t, runID, actor, action, target)
	if err != nil {
		t.Fatal(action, err)
	}
	return cmd
}

func (f *matchFixture) character(t *testing.T, i int) corecharacter.Character {
	t.Helper()
	c, err := f.chars.FindByID(context.Background(), f.actors[i].ID)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *matchFixture) prize(t *testing.T, i int) int {
	t.Helper()
	var total int
	if err := f.db.QueryRowContext(context.Background(), `SELECT COALESCE(SUM(amount),0) FROM character_pending_remittances WHERE character_id = ?`, f.actors[i].ID).Scan(&total); err != nil {
		t.Fatal(err)
	}
	return total
}

func TestRunMatchHandsTheLobbyOffAndPaysTheWinnersOnce(t *testing.T) {
	f := newMatchFixture(t, 1, pvp.ColorRed, pvp.ColorBlue)
	ctx := context.Background()
	entry, err := f.start(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	r := f.run(t, runID)
	a, b := f.actors[0].ID, f.actors[1].ID
	if r.Round != 1 || r.Pool != 2*runBet || r.TargetWins != 1 || r.Actor(a).TeamID != pvp.ColorRed || r.Actor(b).TeamID != pvp.ColorBlue {
		t.Fatal("handoff", r.Round, r.Pool, r.TargetWins)
	}
	if _, err := f.rooms.GetRoom(ctx, f.roomID); !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Fatal("lobby room survived the handoff", err)
	}
	for _, c := range f.actors {
		if claim, err := f.claims.FindCharacterClaim(ctx, c.ID); err != nil || claim != "" {
			t.Fatal("lobby claim survived the handoff", claim, err)
		}
	}
	if again, err := f.start(t, 0); err != nil || again.Command != entry.Command {
		t.Fatal("retried handoff", again, err)
	}
	if f.run(t, runID).Version != r.Version || f.character(t, 0).Money != 1000-runBet {
		t.Fatal("retried handoff changed state")
	}

	// The leader's first かいし was its action.
	if _, err := f.command(t, runID, 0, "attack", b); !errors.Is(err, runhost.ErrCooldown) {
		t.Fatal("leader interval", err)
	}
	foreign := runhost.Command{PlayerID: f.actors[1].PlayerID, ActorID: a, RunID: runID, ID: id.New(), Version: r.Version, Action: "start"}
	if _, err := f.svc.Command(ctx, foreign); !errors.Is(err, runhost.ErrForbidden) {
		t.Fatal("foreign command", err)
	}

	// GET never advances; a standing second team rejects かいし.
	if _, err := f.svc.GetActorRun(ctx, f.actors[0].PlayerID, a); err != nil {
		t.Fatal(err)
	}
	before := *f.run(t, runID)
	f.clock.add(3 * time.Second)
	if _, err := f.command(t, runID, 0, "start", ""); !errors.Is(err, pvp.ErrNotDecided) {
		t.Fatal(err)
	}
	if after := f.run(t, runID); !reflect.DeepEqual(before, *after) {
		t.Fatal("rejected かいし changed the run")
	}

	// An HP-1 target that cannot dodge under the zero RNG.
	f.stage(t, runID, func(r *pvp.Run) { r.Actor(b).HP, r.Actor(b).Status = 1, corebattle.StatusKinju })
	attack := f.send(t, runID, 0, "attack", b)
	if f.run(t, runID).Actor(b).HP > 0 {
		t.Fatal("attack missed")
	}
	f.restart(t)
	if _, err := f.svc.Command(ctx, attack); err != nil {
		t.Fatal("attack replay", err)
	}

	// The fallen loser may call かいし without renewing its action time
	// (_battle.cgi:655–662); the winning color takes the whole pool.
	acted := f.run(t, runID).LastActions[b]
	win := f.send(t, runID, 1, "start", "")
	if got := f.run(t, runID).LastActions[b]; !got.Equal(acted) {
		t.Fatal("fallen かいし renewed the action time", acted, got)
	}
	if f.prize(t, 0) != 2*runBet || f.prize(t, 1) != 0 {
		t.Fatal("prize", f.prize(t, 0), f.prize(t, 1))
	}
	if _, err := f.svc.Command(ctx, win); err != nil || f.prize(t, 0) != 2*runBet {
		t.Fatal("prize replay paid again", err)
	}
	if r := f.run(t, runID); !r.Paid || r.Pool != 0 || r.Round != 1 || r.Wins[pvp.ColorRed] != 1 {
		t.Fatal("payout", r.Paid, r.Pool, r.Round, r.Wins)
	}
	f.clock.add(3 * time.Second)
	if _, err := f.command(t, runID, 0, "start", ""); !errors.Is(err, pvp.ErrMatchFinished) {
		t.Fatal(err)
	}

	stayed := f.character(t, 1)
	f.send(t, runID, 1, "exit", "")
	if left := f.character(t, 1); left.Tired != stayed.Tired+1*3+1 || left.Money != stayed.Money || f.exits.Load() != 1 {
		t.Fatal("exit refunded or skipped fatigue", left.Tired, left.Money)
	}
}

// The first かいし checks two colors, then every color, then the leader
// (vs_player.cgi:75–93); a rejection leaves the lobby intact.
func TestRunFirstKaishiChecks(t *testing.T) {
	f := newMatchFixture(t, 1, pvp.ColorRed, pvp.ColorRed, "")
	if _, err := f.start(t, 1); !errors.Is(err, pvp.ErrTeamsNotConfigured) {
		t.Fatal(err)
	}
	if _, err := f.lobby.SelectTeam(context.Background(), f.actors[2].ID, f.roomID, pvp.ColorRed); err != nil {
		t.Fatal(err)
	}
	if _, err := f.start(t, 0); !errors.Is(err, pvp.ErrNeedAtLeastTwoTeams) {
		t.Fatal(err)
	}
	if _, err := f.lobby.SelectTeam(context.Background(), f.actors[2].ID, f.roomID, pvp.ColorBlue); err != nil {
		t.Fatal(err)
	}
	if _, err := f.start(t, 1); !errors.Is(err, pvp.ErrNotRoomLeader) {
		t.Fatal(err)
	}
	if d, err := f.rooms.GetRoom(context.Background(), f.roomID); err != nil || d.Room.PrizePool != 3*runBet {
		t.Fatal("rejected かいし touched the lobby", d.Room.PrizePool, err)
	}
	if _, err := f.start(t, 0); err != nil {
		t.Fatal(err)
	}
}

// Later かいし: a draw adds no tally, round 10 still starts round 11, and
// round 11 is never counted (vs_player.cgi:101–104,118–120,181).
func TestRunKaishiTallyAndRoundLimit(t *testing.T) {
	f := newMatchFixture(t, 3, pvp.ColorRed, pvp.ColorBlue)
	entry, err := f.start(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	a, b := f.actors[0].ID, f.actors[1].ID
	f.stage(t, runID, func(r *pvp.Run) { r.Actor(a).HP, r.Actor(b).HP = 0, 0 })
	f.send(t, runID, 0, "start", "")
	if r := f.run(t, runID); r.Round != 2 || len(r.Wins) != 0 || r.Actor(a).HP != r.Actor(a).MaxHP {
		t.Fatal("draw", r.Round, r.Wins)
	}
	f.stage(t, runID, func(r *pvp.Run) { r.Round, r.Actor(b).HP = 10, 0 })
	f.send(t, runID, 0, "start", "")
	if r := f.run(t, runID); r.Round != 11 || r.Wins[pvp.ColorRed] != 1 {
		t.Fatal("round 10", r.Round, r.Wins)
	}
	f.stage(t, runID, func(r *pvp.Run) { r.Actor(b).HP = 0 })
	f.clock.add(3 * time.Second)
	if _, err := f.command(t, runID, 0, "start", ""); !errors.Is(err, pvp.ErrRoundLimit) {
		t.Fatal(err)
	}
	if r := f.run(t, runID); r.Wins[pvp.ColorRed] != 1 || r.Pool != 2*runBet {
		t.Fatal("round 11 counted", r.Wins)
	}
}

// Competing first かいし and later かいし apply once with conserved gold.
func TestRunPairedKaishiAppliesOnce(t *testing.T) {
	f := newMatchFixture(t, 2, pvp.ColorRed, pvp.ColorBlue)
	a := f.actors[0]
	var runID atomic.Value
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(int, int) error {
		entry, err := f.svc.EnterParty(context.Background(), a.PlayerID, a.ID, f.roomID)
		if err == nil {
			runID.Store(entry.Command.RunID)
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	id := runID.Load().(string)
	f.stage(t, id, func(r *pvp.Run) { r.Actor(f.actors[1].ID).HP = 0 })
	f.clock.add(3 * time.Second)
	version := f.run(t, id).Version
	stats = testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		c := f.actors[(worker+op)%2]
		_, err := f.svc.Command(context.Background(), runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: id, ID: "start-" + c.ID, Version: version, Action: "start"})
		if errors.Is(err, runhost.ErrConflict) {
			return nil
		}
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	r := f.run(t, id)
	if r.Round != 2 || r.Wins[pvp.ColorRed] != 1 || r.Version != version+1 {
		t.Fatal("かいし applied more than once", r.Round, r.Wins, r.Version)
	}
	if total := f.character(t, 0).Money + f.character(t, 1).Money + r.Pool; total != 2*1000 {
		t.Fatal("gold not conserved", total)
	}
}

// A Colosseum takes up to eight members, and round 1 groups them by color
// (quest.cgi:24, _battle.cgi:1433–1457).
func TestRunHandoffGroupsAFullerRoomByColor(t *testing.T) {
	f := newMatchFixture(t, 1, pvp.ColorRed, pvp.ColorBlue, pvp.ColorRed, pvp.ColorBlue, pvp.ColorRed)
	entry, err := f.start(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	r := f.run(t, entry.Command.RunID)
	var order []string
	for _, id := range r.Roster {
		order = append(order, r.Actor(id).TeamID)
	}
	if want := []string{pvp.ColorRed, pvp.ColorRed, pvp.ColorRed, pvp.ColorBlue, pvp.ColorBlue}; !reflect.DeepEqual(order, want) || r.Pool != 5*runBet {
		t.Fatal(order, r.Pool)
	}
}

func TestRunMatchDoublesFieldDurationAndExercisesTails(t *testing.T) {
	f := newMatchFixture(t, 2, pvp.ColorRed, pvp.ColorBlue)
	ctx := context.Background()
	for i, jobID := range []string{"job-84", "starter"} {
		c := f.actors[i]
		c.JobID, c.SP = jobID, 140
		c.Stats = corecharacter.Stats{HP: 500, MaxHP: 500, MP: 100, MaxMP: 100, Attack: 50, Defense: 50, Agility: 50}
		if err := f.chars.Update(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	entry, err := f.start(t, 0)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	r := f.run(t, runID)
	if !r.Battle.Fields.DoubleTurns || !r.Battle.Fields.ExerciseWithoutResponse {
		t.Fatalf("pvp field flags = %+v", r.Battle.Fields)
	}

	// job-84-entry-4 creates a 5-turn burn field -> 10 in PvP, exercises burn tail (500 - 50 = 450), then ticks to 9.
	f.send(t, runID, 0, "job-84-entry-4", "")
	f.restart(t)
	r = f.run(t, runID)
	if r.Battle.Fields.Temporary != "burn" || r.Battle.Fields.Turns != 9 {
		t.Fatalf("burn after pvp cast = %+v", r.Battle.Fields)
	}
	if a := r.Actor(f.actors[0].ID); a.HP != 450 {
		t.Fatalf("actor 0 HP after pvp burn tail = %d, want 450", a.HP)
	}
	oppBefore := r.Actor(f.actors[1].ID).HP

	// Opponent's defend also exercises burn tail (-50 HP) and ticks turns 9 -> 8.
	f.send(t, runID, 1, "defend", "")
	r = f.run(t, runID)
	if r.Battle.Fields.Turns != 8 {
		t.Fatalf("burn turns after opponent defend = %d, want 8", r.Battle.Fields.Turns)
	}
	if b := r.Actor(f.actors[1].ID); b.HP != oppBefore-50 {
		t.Fatalf("actor 1 HP after pvp burn tail = %d, want %d", b.HP, oppBefore-50)
	}

	// Round reset clears active field turns while retaining PvP field flags.
	f.stage(t, runID, func(r *pvp.Run) { r.Actor(f.actors[1].ID).HP = 0 })
	f.send(t, runID, 0, "start", "")
	r = f.run(t, runID)
	if r.Round != 2 || !r.Battle.Fields.DoubleTurns || !r.Battle.Fields.ExerciseWithoutResponse || r.Battle.Fields.Temporary != "" || r.Battle.Fields.Turns != 0 {
		t.Fatalf("round 2 field state = %+v", r.Battle.Fields)
	}
}
