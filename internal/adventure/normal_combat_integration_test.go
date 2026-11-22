package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// combatFixture serves Command with the production RunCombat.
func combatFixture(t *testing.T, count int) *normalFixture {
	t.Helper()
	f := newNormalFixture(t, count)
	f.cfg.RNG = &midRNG{}
	installRunCombat(t, f)
	return f
}

func installRunCombat(t *testing.T, f *normalFixture) {
	t.Helper()
	f.cfg.Combat, f.explicit = nil, true
	f.restart(t)
}

// midRNG hits ordinary attacks and never rolls crystal drops.
type midRNG struct{ normalRNG }

func (r *midRNG) Float64() float64 { r.calls.Add(1); return .5 }
func (r *midRNG) Intn(n int) int   { r.calls.Add(1); return n / 2 }
func (r *midRNG) IntN(n int) int   { return r.Intn(n) }

// runEnemy is an ordinary starter-job NPC with an empty loadout.
func runEnemy(id string, hp int) corebattle.LifetimeActor {
	return corebattle.LifetimeActor{
		Participant: corebattle.Participant{ID: id, TeamID: "enemies", Name: id, HP: hp, MaxHP: hp, Gear: &corebattle.BattleGearStats{}},
		NPC:         true, Tension: 1, Hit: 95,
		Jobs: corebattle.BattleJobs{JobID: "starter", OldJobID: "starter"},
	}
}

// combatRoom enters a solo run and places enemies on its encounter floor 1.
func combatRoom(t *testing.T, f *normalFixture, c corecharacter.Character, edit func(*adventure.NormalRun), enemies ...corebattle.LifetimeActor) adventure.RunCommand {
	t.Helper()
	c.JobID = "job-01"
	if err := f.chars.Update(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	f.actors[0] = c
	entry := f.enter(t, adventure.CrawlModeNormal)
	cmd := adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: entry.Command.RunID, Version: entry.Version}
	editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
		r.Floor, r.EncounterFloor, r.EncounterStageID = 1, 1, r.Stage.ID
		for _, e := range enemies {
			r.Battle.Actors = append(r.Battle.Actors, e)
			r.Enemies = append(r.Enemies, e.ID)
		}
		a := runActorByID(r, c.ID)
		a.Attack, a.HP, a.MaxHP = 400, 500, 500
		r.LastActions[c.ID] = f.clock.Now()
		if edit != nil {
			edit(r)
		}
	})
	f.clock.add(3 * time.Second)
	return cmd
}

func combatCommand(cmd adventure.RunCommand, id, action, target, input string) adventure.RunCommand {
	cmd.ID, cmd.Action, cmd.TargetID, cmd.Input = id, action, target, input
	return cmd
}

func TestRunCombatResumesOneOperationAndReplays(t *testing.T) {
	f := combatFixture(t, 1)
	foe := runEnemy("foe-a", 5)
	foe.ExperienceReward, foe.GoldReward = 3, 5
	cmd := combatRoom(t, f, f.actors[0], nil, foe, runEnemy("foe-b", 900))
	before, err := f.chars.FindByID(t.Context(), cmd.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	strike := combatCommand(cmd, "strike", "attack", "foe-a", "")
	receipt, err := f.svc.Command(t.Context(), strike)
	if err != nil {
		t.Fatal(err)
	}
	now := f.clock.Now()
	r := readStockRoom(t, f, cmd.RunID)
	if runActorByID(r, "foe-a") != nil || !reflect.DeepEqual(r.Enemies, []string{"foe-b"}) || r.Floor != 1 || r.FloorsCleared != 0 ||
		!r.LastActions["foe-b"].Equal(now) || !r.LastActions[cmd.ActorID].Equal(now) || receipt.Version != cmd.Version+1 {
		t.Fatalf("operation: enemies=%v floor=%d cleared=%d last=%v receipt=%+v", r.Enemies, r.Floor, r.FloorsCleared, r.LastActions, receipt)
	}
	after, err := f.chars.FindByID(t.Context(), cmd.ActorID)
	if err != nil || after.Money != before.Money+5 || after.Experience != before.Experience+3 {
		t.Fatalf("earnings not settled with the receipt: %+v %v", after, err)
	}

	f.restart(t)
	again, err := f.svc.Command(t.Context(), strike)
	if err != nil || !reflect.DeepEqual(again, receipt) {
		t.Fatalf("replay changed receipt: %+v %v", again, err)
	}
	if replayed := readStockRoom(t, f, cmd.RunID); !reflect.DeepEqual(replayed, r) {
		t.Fatal("replay re-executed the operation")
	}
	if c, err := f.chars.FindByID(t.Context(), cmd.ActorID); err != nil || c.Money != after.Money || c.Experience != after.Experience {
		t.Fatalf("replay granted twice: %+v %v", c, err)
	}
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "strike", "attack", "foe-b", "")); !errors.Is(err, adventure.ErrRunConflict) {
		t.Fatalf("receipt input conflict: %v", err)
	}
}

func TestRunCombatRejectionsMutateNothing(t *testing.T) {
	f := combatFixture(t, 1)
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		r.Battle.SkillBans = map[string]int{"job-0-entry-4": 1}
	}, runEnemy("foe", 900))
	f.clock.add(-time.Nanosecond)
	before := readStockRoom(t, f, cmd.RunID)
	for _, tc := range []struct {
		cmd  adventure.RunCommand
		want error
	}{
		{combatCommand(cmd, "early", "attack", "foe", ""), adventure.ErrRunCooldown},
		{combatCommand(cmd, "banned", "defend", "", ""), adventure.ErrRunUnavailable},
		{combatCommand(cmd, "foreign-target", "attack", "nobody", ""), adventure.ErrRunConflict},
		{combatCommand(cmd, "unsupported", "job-50-entry-7", "", ""), adventure.ErrRunUnavailable},
	} {
		if _, err := f.svc.Command(t.Context(), tc.cmd); !errors.Is(err, tc.want) {
			t.Fatalf("%s: %v want %v", tc.cmd.ID, err, tc.want)
		}
		if got, err := f.svc.GetRunReceipt(t.Context(), cmd.PlayerID, cmd.ActorID, cmd.RunID, tc.cmd.ID); err != nil || got != nil {
			t.Fatalf("%s stored receipt: %+v %v", tc.cmd.ID, got, err)
		}
	}
	if after := readStockRoom(t, f, cmd.RunID); !reflect.DeepEqual(before, after) {
		t.Fatal("rejected operation mutated the run")
	}
	f.clock.add(time.Nanosecond) // the exact interval boundary
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "boundary", "attack", "foe", "")); err != nil {
		t.Fatal(err)
	}
}

func TestRunCombatAnswersWithAtMostOneReadyNPC(t *testing.T) {
	f := combatFixture(t, 1)
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		r.LastActions["foe-a"] = f.clock.Now().Add(time.Second)
	}, runEnemy("foe-a", 900), runEnemy("foe-b", 900), runEnemy("foe-c", 900))
	head := readStockRoom(t, f, cmd.RunID).LastActions["foe-a"]
	for range 3 {
		if _, err := f.svc.GetActorRun(t.Context(), cmd.PlayerID, cmd.ActorID); err != nil {
			t.Fatal(err)
		}
	}
	f.clock.add(time.Hour)
	if r := readStockRoom(t, f, cmd.RunID); r.Version != cmd.Version || !r.LastActions["foe-b"].IsZero() {
		t.Fatal("observation or elapsed time acted")
	}
	f.clock.add(-time.Hour)
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "defend", "defend", "", "")); err != nil {
		t.Fatal(err)
	}
	now := f.clock.Now()
	r := readStockRoom(t, f, cmd.RunID)
	if !r.LastActions["foe-a"].Equal(head) || !r.LastActions["foe-b"].Equal(now) || !r.LastActions["foe-c"].IsZero() ||
		!reflect.DeepEqual(r.Enemies, []string{"foe-b", "foe-c", "foe-a"}) {
		t.Fatalf("response order: last=%v enemies=%v", r.LastActions, r.Enemies)
	}
	defended := runActorByID(r, cmd.ActorID).Temporary
	if defended == "" {
		t.Fatal("defend effect was not persisted")
	}
	f.restart(t)
	if reloaded := readStockRoom(t, f, cmd.RunID); runActorByID(reloaded, cmd.ActorID).Temporary != defended {
		t.Fatal("defend effect lost on reload")
	}
}

// _battle.cgi:761–785: the second whitelisted submission runs only while the
// MP left after the first cost strictly exceeds its own cost.
func TestRunCombatSecondSubmissionUsesPostFirstMP(t *testing.T) {
	for _, tc := range []struct {
		mp, wantMP int
		wantTemp   corebattle.TemporaryEffect
	}{
		{13, 7, corebattle.TemporaryReduceMagic},
		{14, 1, corebattle.TemporaryReduceBreath},
	} {
		f := combatFixture(t, 1)
		cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
			r.LastActions["foe"] = f.clock.Now().Add(time.Hour)
			a := runActorByID(r, f.actors[0].ID)
			a.Jobs = corebattle.BattleJobs{JobID: "job-33", SP: 30}
			a.MP, a.MaxMP, a.Extended, a.ExtendedTurns = tc.mp, 50, corebattle.ExtendedAdditional, 2
		}, runEnemy("foe", 900))
		if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "pair", "", "", "＠マジックバリア　＠フバーハ")); err != nil {
			t.Fatal(tc.mp, err)
		}
		a := runActorByID(readStockRoom(t, f, cmd.RunID), cmd.ActorID)
		if a.MP != tc.wantMP || a.Temporary != tc.wantTemp || a.ExtendedTurns != 1 {
			t.Fatalf("mp=%d: got mp=%d temp=%q turns=%d", tc.mp, a.MP, a.Temporary, a.ExtendedTurns)
		}
	}
}

// _battle.cgi:935–937: one level per operation from saved, released and
// direct EXP; the settler credits EXP without further growth.
func TestRunCombatLevelsOnceInTheTail(t *testing.T) {
	for _, tc := range []struct {
		name           string
		exp, reward    int
		bossField      bool
		wantLevel      int
		wantExperience int
	}{
		{"below threshold", 8, 1, false, 1, 9},
		{"exact threshold", 9, 1, false, 2, 10},
		{"two thresholds still one level", 9, 40, false, 2, 49},
		{"EX boss field suppresses", 9, 1, true, 1, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := combatFixture(t, 1)
			c := f.actors[0]
			c.Level, c.Experience = 1, tc.exp
			foe := runEnemy("foe", 5)
			foe.ExperienceReward = tc.reward
			cmd := combatRoom(t, f, c, func(r *adventure.NormalRun) {
				r.Battle.SpecialBossField = tc.bossField
			}, foe)
			strike := combatCommand(cmd, "strike", "attack", "foe", "")
			if _, err := f.svc.Command(t.Context(), strike); err != nil {
				t.Fatal(err)
			}
			got, err := f.chars.FindByID(t.Context(), c.ID)
			if err != nil || got.Level != tc.wantLevel || got.Experience != tc.wantExperience {
				t.Fatalf("level=%d exp=%d err=%v", got.Level, got.Experience, err)
			}
			r := readStockRoom(t, f, cmd.RunID)
			if r.FloorsCleared != 1 || len(r.Enemies) != 0 || r.Floor != 1 {
				t.Fatalf("encounter defeat: cleared=%d enemies=%v floor=%d", r.FloorsCleared, r.Enemies, r.Floor)
			}
			f.restart(t)
			if _, err := f.svc.Command(t.Context(), strike); err != nil {
				t.Fatal(err)
			}
			if again, err := f.chars.FindByID(t.Context(), c.ID); err != nil || again.Level != got.Level || again.Experience != got.Experience {
				t.Fatalf("replay grew again: %+v %v", again, err)
			}
		})
	}
}

func TestRunCombatWriteFailureCommitsNothing(t *testing.T) {
	f := combatFixture(t, 1)
	foe := runEnemy("foe", 5)
	foe.GoldReward = 7
	cmd := combatRoom(t, f, f.actors[0], nil, foe)
	before := readStockRoom(t, f, cmd.RunID)
	money, err := f.chars.FindByID(t.Context(), cmd.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	f.store.failWrite.Store(true)
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "strike", "attack", "foe", "")); err == nil {
		t.Fatal("receipt write should fail")
	}
	f.store.failWrite.Store(false)
	if after := readStockRoom(t, f, cmd.RunID); !reflect.DeepEqual(before, after) {
		t.Fatal("failed write committed run state")
	}
	if c, err := f.chars.FindByID(t.Context(), cmd.ActorID); err != nil || c.Money != money.Money {
		t.Fatalf("failed write committed earnings: %+v %v", c, err)
	}
}

// _battle.cgi:1039–1069: CMP recharge precedes the frenzy strike, whose
// defeat reaches the reward owner and settles with the same receipt.
func TestRunCombatFrenzyDefeatSettlesAfterCMPRecharge(t *testing.T) {
	f := combatFixture(t, 1)
	foe := runEnemy("foe", 1)
	foe.GoldReward = 4
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		a := runActorByID(r, f.actors[0].ID)
		a.CMP, a.MaxCMP, a.CMPCharge, a.CMPNeedCharge = 1, 10, 3, 3
		a.Extended, a.ExtendedTurns = corebattle.ExtendedFrenzy, 2
	}, foe)
	before, err := f.chars.FindByID(t.Context(), cmd.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "frenzy", "defend", "", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	a := runActorByID(r, cmd.ActorID)
	after, err := f.chars.FindByID(t.Context(), cmd.ActorID)
	if err != nil || a.CMP != 2 || a.CMPCharge != 0 || a.ExtendedTurns != 1 || len(r.Enemies) != 0 || r.FloorsCleared != 1 || after.Money != before.Money+4 {
		t.Fatalf("cmp=%d charge=%d turns=%d enemies=%v money=%d->%d err=%v", a.CMP, a.CMPCharge, a.ExtendedTurns, r.Enemies, before.Money, after.Money, err)
	}
}

// Advance creates the stage24 floor10 encounter, then caps the party through
// the shared reset owner before any response (_battle.cgi:1485–1530).
func TestRunCombatBossFieldCapsOnAdvance(t *testing.T) {
	f := combatFixture(t, 1)
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		stage, err := f.cfg.Stages.FindByID("stage-24")
		if err != nil {
			t.Fatal(err)
		}
		r.Stage, r.Floor, r.FloorsCleared, r.EncounterFloor = stage, 9, 9, 9
		a := runActorByID(r, f.actors[0].ID)
		a.MaxHP, a.HP = 2000, 2000
	})
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "advance", "advance", "", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	a := runActorByID(r, cmd.ActorID)
	if r.Floor != 10 || len(r.Enemies) == 0 || !r.Battle.SpecialBossField || a.MaxHP != 999 || a.HP != 2000 {
		t.Fatalf("floor=%d enemies=%v field=%v maxHP=%d hp=%d", r.Floor, r.Enemies, r.Battle.SpecialBossField, a.MaxHP, a.HP)
	}
}

type failingRunCharacters struct {
	adventure.RunCharacters
	fail bool
}

func (c *failingRunCharacters) Update(ctx context.Context, v corecharacter.Character) error {
	if c.fail {
		return errors.New("character write failed")
	}
	return c.RunCharacters.Update(ctx, v)
}

// An external failure inside the operation discards the player step, the NPC
// response and the level-up; the retried command then applies them once.
func TestRunCombatExecutionFailureDiscardsOperation(t *testing.T) {
	f := combatFixture(t, 1)
	chars := &failingRunCharacters{RunCharacters: f.cfg.Characters, fail: true}
	f.cfg.Characters = chars
	f.restart(t)
	c := f.actors[0]
	c.Level, c.Experience = 1, 9
	foe := runEnemy("foe", 5)
	foe.ExperienceReward = 1
	cmd := combatRoom(t, f, c, nil, foe, runEnemy("other", 900))
	before := readStockRoom(t, f, cmd.RunID)
	strike := combatCommand(cmd, "strike", "attack", "foe", "")
	if _, err := f.svc.Command(t.Context(), strike); err == nil {
		t.Fatal("character write failure was absorbed")
	}
	if after := readStockRoom(t, f, cmd.RunID); !reflect.DeepEqual(before, after) {
		t.Fatal("failed operation committed run state")
	}
	if got, err := f.chars.FindByID(t.Context(), c.ID); err != nil || got.Level != 1 || got.Experience != 9 {
		t.Fatalf("failed operation committed progression: %+v %v", got, err)
	}
	chars.fail = false
	if _, err := f.svc.Command(t.Context(), strike); err != nil {
		t.Fatal(err)
	}
	if got, err := f.chars.FindByID(t.Context(), c.ID); err != nil || got.Level != 2 || got.Experience != 10 {
		t.Fatalf("retry: %+v %v", got, err)
	}
}

// #2554: a stock release credits EXP without growth (get_stock); exit has no
// tail, while advance reaches the tail's single lv_up (_battle.cgi:912,935–937,1208).
func TestNormalRunStockReleaseLevelsOnlyInTheTail(t *testing.T) {
	for _, tc := range []struct {
		action    string
		wantLevel int
	}{{"exit", 1}, {"advance", 2}} {
		t.Run(tc.action, func(t *testing.T) {
			f := combatFixture(t, 1)
			c := f.actors[0]
			c.Level, c.Experience, c.JobID = 1, 0, "job-01"
			if err := f.chars.Update(t.Context(), c); err != nil {
				t.Fatal(err)
			}
			entry := f.enter(t, adventure.CrawlModeNormal)
			cmd := adventure.RunCommand{PlayerID: c.PlayerID, ActorID: c.ID, RunID: entry.Command.RunID, Version: entry.Version}
			editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
				p := r.Recipients[c.ID]
				p.Pending.EXP = 49
				r.Recipients[c.ID] = p
			})
			f.clock.add(3 * time.Second)
			release := combatCommand(cmd, "release", tc.action, "", "")
			if _, err := f.svc.Command(t.Context(), release); err != nil {
				t.Fatal(err)
			}
			got, err := f.chars.FindByID(t.Context(), c.ID)
			if err != nil || got.Level != tc.wantLevel || got.Experience != 49 {
				t.Fatalf("level=%d exp=%d err=%v", got.Level, got.Experience, err)
			}
			if _, err := f.svc.Command(t.Context(), release); err != nil {
				t.Fatal(err)
			}
			if again, err := f.chars.FindByID(t.Context(), c.ID); err != nil || again.Level != got.Level || again.Experience != 49 {
				t.Fatalf("replay changed progression: %+v %v", again, err)
			}
		})
	}
}
